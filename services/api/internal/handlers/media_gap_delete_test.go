package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/services/mediagap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestMediaGapDeleteHTTP 使用真实 handler/service 和 SQL mock 校验删除协议，无外部服务。
func TestMediaGapDeleteHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body, status string
		code                             int
	}{
		{"single", http.MethodDelete, "/gaps/a", "", "INGESTED", 200},
		{"batch", http.MethodPost, "/gaps/batch-delete", `{"ids":["a"]}`, "IGNORED", 200},
		{"conflict", http.MethodDelete, "/gaps/a", "", "REQUESTED", 409},
		{"absent", http.MethodDelete, "/gaps/a", "", "", 404},
		{"invalid-json", http.MethodPost, "/gaps/batch-delete", `{`, "", 400},
		{"empty", http.MethodPost, "/gaps/batch-delete", `{"ids":[]}`, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
			if err != nil {
				t.Fatal(err)
			}
			original := db.DB
			db.DB = database
			t.Cleanup(func() {
				db.DB = original
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Error(err)
				}
				sqlDB.Close()
			})
			if tc.code != 400 {
				mock.ExpectBegin()
				rows := sqlmock.NewRows([]string{"id", "status"})
				if tc.status != "" {
					rows.AddRow("a", tc.status)
				}
				mock.ExpectQuery(`SELECT .* FROM "media_gaps" .* FOR UPDATE`).WillReturnRows(rows)
				if tc.code == 200 {
					mock.ExpectExec(`DELETE FROM "media_gaps"`).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				} else {
					mock.ExpectRollback()
				}
			}
			handler := &MediaGapHandler{service: &mediagap.Service{}}
			router := gin.New()
			router.DELETE("/gaps/:id", handler.DeleteMediaGap)
			router.POST("/gaps/batch-delete", handler.BatchDeleteMediaGaps)
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.code {
				t.Fatalf("code=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if tc.code == 200 {
				var response struct {
					Data struct {
						DeletedCount int `json:"deletedCount"`
					} `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.DeletedCount != 1 {
					t.Fatalf("unexpected response: %s", recorder.Body.String())
				}
			}
		})
	}
}
