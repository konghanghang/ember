package mediagap

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	moviepilotint "github.com/konghang/ember/backend/internal/integrations/moviepilot"
	"github.com/konghang/ember/backend/internal/models"
	subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

// newGapSQLMock 使用真实 GORM SQL 生成配合假数据库，禁止访问外部依赖。
func newGapSQLMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	swapGlobalDB(t, database)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		sqlDB.Close()
	})
	return mock
}

// TestExternalResultCannotOverwriteTerminalStatus 暂停外部请求，在其间模拟数据库终态，验证状态条件与冲突返回。
func TestExternalResultCannotOverwriteTerminalStatus(t *testing.T) {
	for _, operation := range []string{"search", "dispatch", "dispatch-failure"} {
		for _, transition := range []string{"ignore", "ingest"} {
			t.Run(operation+"/"+transition, func(t *testing.T) {
				mock := newGapSQLMock(t)
				gap := models.MediaGap{ID: "gap", Status: models.MediaGapStatusMissing, TmdbID: "123", Season: 1, Episode: 2}
				restore := swapLoadGapByIDFunc(gap)
				defer restore()
				entered, release := make(chan struct{}), make(chan struct{})
				client := &stubGapMoviePilotClient{
					searchFn: func(moviepilotint.GapSearchRequest) (*moviepilotint.GapSearchResponse, error) {
						close(entered)
						<-release
						return &moviepilotint.GapSearchResponse{}, nil
					},
					dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
						close(entered)
						<-release
						if operation == "dispatch-failure" {
							return nil, errors.New("offline")
						}
						return &moviepilotint.GapDispatchResponse{}, nil
					},
				}
				if operation != "search" {
					mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				result := make(chan error, 1)
				go func() {
					svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: client}
					var err error
					if operation == "search" {
						_, err = svc.SearchGap(context.Background(), "gap")
					} else {
						_, err = svc.DispatchGap(context.Background(), "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
					}
					result <- err
				}()
				<-entered
				svc := &Service{}
				rows := sqlmock.NewRows([]string{"id", "status", "tmdb_id", "season", "episode"}).AddRow("gap", models.MediaGapStatusMissing, "123", 1, 2)
				mock.ExpectQuery(`SELECT .* FROM "media_gaps"`).WillReturnRows(rows)
				if transition == "ignore" {
					mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = \$[0-9]+$`).WillReturnResult(sqlmock.NewResult(0, 1))
					if _, err := svc.Ignore(context.Background(), "gap", "manual"); err != nil {
						t.Fatal(err)
					}
				} else {
					mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = \$[0-9]+ AND status <> \$[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 1))
					count, err := svc.MarkIngestedByWebhook(context.Background(), subscriptionpkg.SubscriptionIngestWebhookPayload{ItemType: "Episode", TmdbID: "123", Season: 1, Episode: 2})
					if err != nil || count != 1 {
						t.Fatalf("webhook count=%d err=%v", count, err)
					}
				}
				mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = \$[0-9]+ AND status = \$[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
				close(release)
				if err := <-result; err == nil || err.Error() != "缺集工单状态已变化，请刷新后重试" {
					t.Fatalf("expected state conflict, got %v", err)
				}
			})
		}
	}
}

// TestWebhookConditionalWriteCountsActualRows 模拟读取后人工忽略导致零行回写，计数不得报告核销成功。
func TestWebhookConditionalWriteCountsActualRows(t *testing.T) {
	mock := newGapSQLMock(t)
	mock.ExpectQuery(`SELECT .* FROM "media_gaps"`).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "season", "episode"}).AddRow("gap", models.MediaGapStatusMissing, 1, 2))
	mock.ExpectExec(`UPDATE "media_gaps" SET .*"ingested_at"=COALESCE\(ingested_at, \$[0-9]+\).* WHERE id = \$[0-9]+ AND status <> \$[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 0))
	count, err := (&Service{}).MarkIngestedByWebhook(context.Background(), subscriptionpkg.SubscriptionIngestWebhookPayload{ItemType: "Episode", TmdbID: "123", Season: 1, Episode: 2})
	if err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

// TestCleanupInactiveSeasonCountsOnlyChangedRows 并发人工忽略后系统清理不得虚报更新或覆盖人工原因。
func TestCleanupInactiveSeasonCountsOnlyChangedRows(t *testing.T) {
	mock := newGapSQLMock(t)
	mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = \$[0-9]+ AND status IN \(\$[0-9]+,\$[0-9]+\)`).WillReturnResult(sqlmock.NewResult(0, 0))
	count, err := (&Service{}).cleanupInactiveSeasonGaps(context.Background(), "series", "name", time.Now(), map[string]*models.MediaGap{"1:2": {ID: "gap", Status: models.MediaGapStatusMissing}}, nil)
	if err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

// TestScanMetadataNeverWritesStaleStatus 锁定扫描只刷新元数据；空状态被其他请求推进后不能重置。
func TestScanMetadataNeverWritesStaleStatus(t *testing.T) {
	for _, status := range []models.MediaGapStatus{models.MediaGapStatusMissing, models.MediaGapStatusRequested, ""} {
		t.Run(string(status), func(t *testing.T) {
			mock := newGapSQLMock(t)
			if status == "" {
				mock.ExpectExec(`UPDATE "media_gaps" SET "status"=\$1,"updated_at"=\$2 WHERE id = \$3 AND status = \$4`).WithArgs(models.MediaGapStatusMissing, sqlmock.AnyArg(), "gap", "").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec(`UPDATE "media_gaps" SET "air_date"=\$1,"emby_series_id"=\$2,"last_scanned_at"=\$3,"series_name"=\$4,"updated_at"=\$5 WHERE id = \$6`).WillReturnResult(sqlmock.NewResult(0, 1))
			if err := updateScannedGapMetadata(context.Background(), &models.MediaGap{ID: "gap", Status: status, SeriesName: "updated"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestExternalSuccessReturnsCurrentRecord 验证成功后重读权威记录，不能把旧状态或旧快照重新装入响应。
func TestExternalSuccessReturnsCurrentRecord(t *testing.T) {
	for _, operation := range []string{"search", "dispatch"} {
		t.Run(operation, func(t *testing.T) {
			mock := newGapSQLMock(t)
			original := loadGapByIDFunc
			defer func() { loadGapByIDFunc = original }()
			reads := 0
			loadGapByIDFunc = func(context.Context, string) (models.MediaGap, error) {
				reads++
				if reads == 1 {
					return models.MediaGap{ID: "gap", Status: models.MediaGapStatusMissing}, nil
				}
				return models.MediaGap{ID: "gap", Status: models.MediaGapStatusIgnored}, nil
			}
			if operation == "dispatch" {
				mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = \$[0-9]+ AND status = \$[0-9]+`).WillReturnResult(sqlmock.NewResult(0, 1))
			svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{
				searchFn: func(moviepilotint.GapSearchRequest) (*moviepilotint.GapSearchResponse, error) {
					return &moviepilotint.GapSearchResponse{}, nil
				},
				dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
					return &moviepilotint.GapDispatchResponse{}, nil
				},
			}}
			var dto *MediaGapDTO
			var err error
			if operation == "search" {
				dto, err = svc.SearchGap(context.Background(), "gap")
			} else {
				dto, err = svc.DispatchGap(context.Background(), "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
			}
			if err != nil || dto.Status != models.MediaGapStatusIgnored || dto.SearchSnapshot != nil || dto.DispatchSnapshot != nil || reads != 2 {
				t.Fatalf("dto=%+v err=%v reads=%d", dto, err, reads)
			}
		})
	}
}

// TestScanMetadataWriteErrors 数据库失败必须向扫描调用方报告，不能将未持久化元数据计为成功。
func TestScanMetadataWriteErrors(t *testing.T) {
	for _, status := range []models.MediaGapStatus{"", models.MediaGapStatusMissing} {
		t.Run(string(status), func(t *testing.T) {
			mock := newGapSQLMock(t)
			failure := errors.New("database write failed")
			mock.ExpectExec(`UPDATE "media_gaps" SET`).WillReturnError(failure)
			if err := updateScannedGapMetadata(context.Background(), &models.MediaGap{ID: "gap", Status: status}); !errors.Is(err, failure) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

// TestDispatchFailurePersistsSafeError 失败回写成功后仍返回上游安全错误，且不得存储原始网络细节。
func TestDispatchFailurePersistsSafeError(t *testing.T) {
	mock := newGapSQLMock(t)
	restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusSearched})
	defer restore()
	mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "media_gaps" SET "last_dispatch_error"=\$1,"status"=\$2,"updated_at"=\$3 WHERE id = \$4 AND status = \$5 AND dispatch_snapshot = \$6`).WithArgs("下发结果待确认，请先核对 MoviePilot 任务", models.MediaGapStatusRequested, sqlmock.AnyArg(), "gap", models.MediaGapStatusRequested, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		return nil, errors.New("private network detail")
	}}}
	_, err := svc.DispatchGap(context.Background(), "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
	if err == nil || err.Error() != "下发候选资源失败: upstream moviepilot unavailable" {
		t.Fatalf("got %v", err)
	}
}

// TestGroupedOpenFilterBeforePagination 验证默认查询先过滤终态；显式历史筛选仍可读取。
func TestGroupedOpenFilterBeforePagination(t *testing.T) {
	for _, status := range []string{"", "OPEN", "ALL", "INGESTED", "IGNORED", "DISPATCH_FAILED"} {
		t.Run(status, func(t *testing.T) {
			mock := newGapSQLMock(t)
			predicate := ""
			if status == "" || status == "OPEN" {
				predicate = ` WHERE status IN \(.*\)`
			} else if status != "ALL" {
				predicate = ` WHERE status = .*`
			}
			countQuery := mock.ExpectQuery(`SELECT count\(\*\) FROM "media_gaps"` + predicate + `$`)
			dataQuery := mock.ExpectQuery(`SELECT \* FROM "media_gaps"` + predicate + ` ORDER BY`)
			if status == "" || status == "OPEN" {
				countQuery.WithArgs(models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed)
				dataQuery.WithArgs(models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed)
			} else if status != "ALL" {
				countQuery.WithArgs(status)
				dataQuery.WithArgs(status)
			}
			countQuery.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			dataQuery.WillReturnRows(sqlmock.NewRows([]string{"id"}))
			result, err := (&Service{}).ListGrouped(context.Background(), GroupedListRequest{Status: status, Page: 3})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != 0 || len(result.Data) != 0 || result.Page != 1 {
				t.Fatalf("invalid empty page: %+v", result)
			}
		})
	}
}

// TestRequestedGapRequiresExplicitRetry 已下发工单不能被普通重复请求再次发送到上游。
func TestRequestedGapRequiresExplicitRetry(t *testing.T) {
	restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusRequested})
	defer restore()
	called := false
	svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		called = true
		return nil, errors.New("unexpected dispatch")
	}}}
	newGapSQLMock(t)
	_, err := svc.DispatchGap(context.Background(), "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
	if err == nil || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

// fakeDispatchLock 仅用于状态流转测试；锁的数据库语义由独立 SQL mock 用例覆盖。
func fakeDispatchLock(context.Context, string) (func(), error) { return func() {}, nil }

// TestDispatchSerializesAcrossServices 同一工单并发请求只有一个会访问 MoviePilot。
func TestDispatchSerializesAcrossServices(t *testing.T) {
	mock := newGapSQLMock(t)
	restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusMissing})
	defer restore()
	mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
	var lock sync.Mutex
	acquire := func(context.Context, string) (func(), error) {
		if !lock.TryLock() {
			return nil, ErrMediaGapStateConflict
		}
		return lock.Unlock, nil
	}
	entered, release := make(chan struct{}), make(chan struct{})
	client := &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		close(entered)
		<-release
		return &moviepilotint.GapDispatchResponse{}, nil
	}}
	first := &Service{acquireDispatchLock: acquire, moviepilot: client}
	second := &Service{acquireDispatchLock: acquire, moviepilot: client}
	req := DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}}
	done := make(chan error, 1)
	go func() { _, err := first.DispatchGap(context.Background(), "gap", req); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("first dispatch stopped: %v", err)
	case <-time.After(time.Second):
		t.Fatal("dispatch did not start")
	}
	_, err := second.DispatchGap(context.Background(), "gap", req)
	close(release)
	if !errors.Is(err, ErrMediaGapStateConflict) {
		t.Fatalf("second dispatch: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestDispatchClaimFailureNeverCallsUpstream 写意图失败或状态被抢先修改时，不能发出外部请求。
func TestDispatchClaimFailureNeverCallsUpstream(t *testing.T) {
	for _, databaseFailure := range []bool{false, true} {
		mock := newGapSQLMock(t)
		restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusMissing})
		expectation := mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`)
		if databaseFailure {
			expectation.WillReturnError(errors.New("database unavailable"))
		} else {
			expectation.WillReturnResult(sqlmock.NewResult(0, 0))
		}
		called := false
		svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
			called = true
			return nil, nil
		}}}
		_, err := svc.DispatchGap(context.Background(), "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
		restore()
		if err == nil || called {
			t.Fatalf("err=%v called=%t", err, called)
		}
	}
}

// TestDispatchRetryRequiresCurrentVersion 显式重发仍须匹配当前版本，迟到的重复重试不得再次发送。
func TestDispatchRetryRequiresCurrentVersion(t *testing.T) {
	now := time.Now().UTC()
	for _, stale := range []bool{false, true} {
		mock := newGapSQLMock(t)
		restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusRequested, UpdatedAt: now})
		expected := now
		if stale {
			expected = now.Add(-time.Second)
		} else {
			mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
		}
		called := false
		svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
			called = true
			return &moviepilotint.GapDispatchResponse{}, nil
		}}}
		_, err := svc.DispatchGap(context.Background(), "gap", DispatchRequest{Retry: true, ExpectedUpdatedAt: expected, Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}})
		restore()
		if stale && (!errors.Is(err, ErrMediaGapStateConflict) || called) {
			t.Fatalf("stale retry err=%v called=%t", err, called)
		}
		if !stale && (err != nil || !called) {
			t.Fatalf("fresh retry err=%v called=%t", err, called)
		}
	}
}

// TestGroupedPageClampsWithOpenRecords 收口使页数缩小时仍返回最后一页工单及一致摘要。
func TestGroupedPageClampsWithOpenRecords(t *testing.T) {
	mock := newGapSQLMock(t)
	mock.ExpectQuery(`SELECT count`).WithArgs(models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT \* FROM "media_gaps" WHERE status IN`).WithArgs(models.MediaGapStatusMissing, models.MediaGapStatusSearched, models.MediaGapStatusRequested, models.MediaGapStatusDispatchFailed).WillReturnRows(sqlmock.NewRows([]string{"id", "tmdb_id", "series_name", "season", "episode", "status"}).AddRow("a", "1", "A", 1, 1, "REQUESTED").AddRow("b", "2", "B", 1, 1, "DISPATCH_FAILED"))
	result, err := (&Service{}).ListGrouped(context.Background(), GroupedListRequest{Page: 3, PageSize: 1, Sort: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Page != 2 || result.Total != 2 || result.ItemTotal != 2 || len(result.Data) != 1 || result.Data[0].TmdbID != "2" || result.Summary.RequestedCount != 1 || result.Summary.DispatchFailedCount != 1 {
		t.Fatalf("unexpected page: %+v", result)
	}
}

// TestDispatchCanceledRequestStillFinalizes 客户端取消不能让已受理请求丢失结果回写。
func TestDispatchCanceledRequestStillFinalizes(t *testing.T) {
	mock := newGapSQLMock(t)
	restore := swapLoadGapByIDFunc(models.MediaGap{ID: "gap", Status: models.MediaGapStatusMissing})
	defer restore()
	mock.ExpectExec(`UPDATE "media_gaps" SET .* WHERE id = .* AND status = .* AND dispatch_snapshot = .*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "media_gaps" SET "last_dispatch_error"=\$1,"status"=\$2,"updated_at"=\$3 WHERE id = \$4 AND status = \$5 AND dispatch_snapshot = \$6`).WithArgs(nil, models.MediaGapStatusRequested, sqlmock.AnyArg(), "gap", models.MediaGapStatusRequested, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := &Service{acquireDispatchLock: fakeDispatchLock, moviepilot: &stubGapMoviePilotClient{dispatchFn: func(moviepilotint.GapDispatchRequest) (*moviepilotint.GapDispatchResponse, error) {
		cancel()
		return &moviepilotint.GapDispatchResponse{}, nil
	}}}
	if _, err := svc.DispatchGap(ctx, "gap", DispatchRequest{Candidate: SearchCandidate{Payload: map[string]interface{}{"title": "candidate"}}}); err != nil {
		t.Fatal(err)
	}
}
