package mediagap

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/konghang/ember/backend/internal/models"
)

// TestDeleteClosedGapsAtomic 验证单条、批量、状态限制、并发缺失及数据库失败均为原子操作。
func TestDeleteClosedGapsAtomic(t *testing.T) {
	cases := []struct {
		name      string
		statuses  []models.MediaGapStatus
		deleted   int64
		deleteErr bool
		wantErr   error
	}{
		{"ingested", []models.MediaGapStatus{models.MediaGapStatusIngested}, 1, false, nil},
		{"ignored", []models.MediaGapStatus{models.MediaGapStatusIgnored}, 1, false, nil},
		{"batch", []models.MediaGapStatus{models.MediaGapStatusIngested, models.MediaGapStatusIgnored}, 2, false, nil},
		{"missing", []models.MediaGapStatus{models.MediaGapStatusMissing}, 0, false, ErrMediaGapDeleteState},
		{"searched", []models.MediaGapStatus{models.MediaGapStatusSearched}, 0, false, ErrMediaGapDeleteState},
		{"requested", []models.MediaGapStatus{models.MediaGapStatusRequested}, 0, false, ErrMediaGapDeleteState},
		{"dispatch-failed", []models.MediaGapStatus{models.MediaGapStatusDispatchFailed}, 0, false, ErrMediaGapDeleteState},
		{"mixed", []models.MediaGapStatus{models.MediaGapStatusIgnored, models.MediaGapStatusRequested}, 0, false, ErrMediaGapDeleteState},
		{"not-found", nil, 0, false, ErrMediaGapNotFound},
		{"affected-mismatch", []models.MediaGapStatus{models.MediaGapStatusIgnored}, 0, false, ErrMediaGapStateConflict},
		{"db-failure", []models.MediaGapStatus{models.MediaGapStatusIngested}, 0, true, errors.New("database unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := newGapSQLMock(t)
			ids := []string{"a"}
			if len(tc.statuses) == 2 {
				ids = append(ids, "b")
			}
			mock.ExpectBegin()
			rows := sqlmock.NewRows([]string{"id", "status"})
			for i, status := range tc.statuses {
				rows.AddRow(ids[i], status)
			}
			mock.ExpectQuery(`SELECT .* FROM "media_gaps" WHERE id IN .* ORDER BY id ASC FOR UPDATE`).WillReturnRows(rows)
			if tc.wantErr == nil || tc.deleteErr || tc.wantErr == ErrMediaGapStateConflict {
				deletion := mock.ExpectExec(`DELETE FROM "media_gaps" WHERE id IN .* AND status IN .*`)
				if len(ids) == 1 {
					deletion.WithArgs("a", models.MediaGapStatusIngested, models.MediaGapStatusIgnored)
				} else {
					deletion.WithArgs("a", "b", models.MediaGapStatusIngested, models.MediaGapStatusIgnored)
				}
				if tc.deleteErr {
					deletion.WillReturnError(tc.wantErr)
				} else {
					deletion.WillReturnResult(sqlmock.NewResult(0, tc.deleted))
				}
			}
			if tc.wantErr == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			count, err := (&Service{}).DeleteClosedGaps(context.Background(), ids)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && count != 0 {
				t.Fatalf("failed transaction returned count=%d", count)
			}
			if tc.wantErr == nil && count != int64(len(ids)) {
				t.Fatalf("count=%d", count)
			}
		})
	}
}

// TestDeleteClosedGapsValidation 空列表、空 ID 和过量请求不得到达数据库；重复 ID 只删除一次。
func TestDeleteClosedGapsValidation(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {" "}, {"a", ""}, make([]string, 101)} {
		if _, err := (&Service{}).DeleteClosedGaps(context.Background(), ids); !errors.Is(err, ErrMediaGapDeleteIDs) {
			t.Fatalf("ids=%v err=%v", ids, err)
		}
	}
	mock := newGapSQLMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "media_gaps" WHERE id IN .* FOR UPDATE`).WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("a", "IGNORED"))
	mock.ExpectExec(`DELETE FROM "media_gaps"`).WithArgs("a", models.MediaGapStatusIngested, models.MediaGapStatusIgnored).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if count, err := (&Service{}).DeleteClosedGaps(context.Background(), []string{" a ", "a"}); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
