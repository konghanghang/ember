package directplay

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

// TestIntegrationTouchSucceededSamplesLatestTask checks real PostgreSQL update
// semantics, including deterministic selection and concurrent no-op row versions.
func TestIntegrationTouchSucceededSamplesLatestTask(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	source, err := accounts.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	if err != nil {
		t.Fatal(err)
	}
	playback, err := accounts.LoadActiveCredentialByRole(context.Background(), models.P115AccountRolePlayback)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	old := at.Add(-time.Hour)
	fileID, pickCode := "fixture-file", "fixture-pick"
	for _, id := range []string{"sample_a", "sample_b"} {
		task := models.PlaybackTransferTask{ID: id, SourceAccountID: source.Credential.AccountID, PlaybackAccountID: playback.Credential.AccountID,
			SHA1: directPlaySourceSHA1, Size: 1024, FileName: "fixture.mkv", TargetParentID: playback.TargetParentID,
			Status: models.PlaybackTransferTaskStatusSucceeded, TargetFileID: &fileID, TargetPickCode: &pickCode,
			StartedAt: old, CompletedAt: &old, LastAccessedAt: &old, CreatedAt: old, UpdatedAt: old}
		if err := database.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	store := &gormTaskStore{db: database}
	touch := func(at time.Time) error {
		return store.TouchSucceeded(context.Background(), playback.Credential.AccountID, directPlaySourceSHA1, 1024, at)
	}
	load := func(id string) models.PlaybackTransferTask {
		t.Helper()
		var task models.PlaybackTransferTask
		if err := database.Where("id = ?", id).First(&task).Error; err != nil {
			t.Fatal(err)
		}
		return task
	}
	rowVersion := func() string {
		t.Helper()
		var value string
		if err := database.Raw("SELECT ctid::text FROM playback_transfer_tasks WHERE id = ?", "sample_b").Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	if err := touch(at); err != nil {
		t.Fatal(err)
	}
	if !load("sample_b").LastAccessedAt.Equal(at) || !load("sample_a").LastAccessedAt.Equal(old) {
		t.Fatal("wrong latest task selected")
	}
	version := rowVersion()
	var workers sync.WaitGroup
	errorsCh := make(chan error, 8)
	for range 8 {
		workers.Add(1)
		go func() { defer workers.Done(); errorsCh <- touch(at.Add(10 * time.Second)) }()
	}
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, moment := range []time.Time{at.Add(-time.Minute), at.Add(59 * time.Second)} {
		if err := touch(moment); err != nil {
			t.Fatal(err)
		}
	}
	if rowVersion() != version {
		t.Fatal("recent or older access physically rewrote the row")
	}
	if err := touch(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !load("sample_b").LastAccessedAt.Equal(at.Add(time.Minute)) {
		t.Fatal("window boundary failed to refresh")
	}
	version = rowVersion()
	for _, input := range []struct {
		account string
		size    int64
	}{{"missing", 1024}, {playback.Credential.AccountID, 2048}} {
		if err := store.TouchSucceeded(context.Background(), input.account, directPlaySourceSHA1, input.size, at.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if rowVersion() != version {
		t.Fatal("different account or content affected the task")
	}
}
