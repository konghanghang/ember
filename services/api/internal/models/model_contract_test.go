package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestModelTableNames(t *testing.T) {
	tests := []struct {
		name  string
		model interface{}
		want  string
	}{
		{name: "bot pending reject request", model: BotPendingRejectRequest{}, want: "bot_pending_reject_requests"},
		{name: "bot runtime lock", model: BotRuntimeLock{}, want: "bot_runtime_locks"},
		{name: "client blacklist", model: ClientBlacklist{}, want: "client_blacklists"},
		{name: "device action", model: DeviceAction{}, want: "device_actions"},
		{name: "email verification", model: EmailVerification{}, want: "email_verifications"},
		{name: "emby access token", model: EmbyAccessToken{}, want: "emby_access_tokens"},
		{name: "failed emby async op", model: FailedEmbyAsyncOp{}, want: "failed_emby_async_ops"},
		{name: "media gap", model: MediaGap{}, want: "media_gaps"},
		{name: "media gap scan", model: MediaGapScan{}, want: "media_gap_scans"},
		{name: "plan group media library", model: PlanGroupMediaLibrary{}, want: "plan_group_media_libraries"},
		{name: "plan group emby policy template", model: PlanGroupEmbyPolicyTemplate{}, want: "plan_group_emby_policy_templates"},
		{name: "user media library preference", model: UserMediaLibraryPreference{}, want: "user_media_library_preferences"},
		{name: "emby policy sync batch", model: EmbyPolicySyncBatch{}, want: "emby_policy_sync_batches"},
		{name: "emby policy sync task", model: EmbyPolicySyncTask{}, want: "emby_policy_sync_tasks"},
		{name: "media quality cache", model: MediaQualityCache{}, want: "media_quality_caches"},
		{name: "payment", model: Payment{}, want: "payments"},
		{name: "p115 account", model: P115Account{}, want: "p115_accounts"},
		{name: "playback transfer task", model: PlaybackTransferTask{}, want: "playback_transfer_tasks"},
		{name: "plan", model: Plan{}, want: "plans"},
		{name: "plan group", model: PlanGroup{}, want: "plan_groups"},
		{name: "playback ranking", model: PlaybackRanking{}, want: "playback_rankings"},
		{name: "redemption", model: Redemption{}, want: "redemptions"},
		{name: "redemption code", model: RedemptionCode{}, want: "redemption_codes"},
		{name: "setting", model: Setting{}, want: "settings"},
		{name: "stripe webhook event", model: StripeWebhookEvent{}, want: "stripe_webhook_events"},
		{name: "subscription", model: Subscription{}, want: "subscriptions"},
		{name: "subscription admin notification", model: SubscriptionAdminNotification{}, want: "subscription_admin_notifications"},
		{name: "telegram bind code", model: TelegramBindCode{}, want: "telegram_bind_codes"},
		{name: "tv calendar source", model: TVCalendarSource{}, want: "tv_calendar_sources"},
		{name: "tv calendar item", model: TVCalendarItem{}, want: "tv_calendar_items"},
		{name: "tv calendar subscription", model: TVCalendarSubscription{}, want: "tv_calendar_subscriptions"},
		{name: "tmdb cache", model: TMDBCache{}, want: "tmdb_cache"},
		{name: "user", model: User{}, want: "users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := reflect.ValueOf(tt.model).MethodByName("TableName")
			if !method.IsValid() {
				t.Fatalf("%T does not expose TableName", tt.model)
			}
			got := method.Call(nil)[0].String()
			if got != tt.want {
				t.Fatalf("TableName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBeforeCreateGeneratesCUIDWhenIDMissing(t *testing.T) {
	models := []interface{}{
		&BotPendingRejectRequest{},
		&ClientBlacklist{},
		&DeviceAction{},
		&EmailVerification{},
		&EmbyAccessToken{},
		&FailedEmbyAsyncOp{},
		&MediaGap{},
		&MediaGapScan{},
		&PlanGroupMediaLibrary{},
		&UserMediaLibraryPreference{},
		&EmbyPolicySyncBatch{},
		&EmbyPolicySyncTask{},
		&MediaQualityCache{},
		&Payment{},
		&P115Account{},
		&PlaybackTransferTask{},
		&Plan{},
		&PlaybackRanking{},
		&Redemption{},
		&RedemptionCode{},
		&Subscription{},
		&SubscriptionAdminNotification{},
		&TelegramBindCode{},
		&TVCalendarSource{},
		&TVCalendarItem{},
		&TVCalendarSubscription{},
		&TMDBCache{},
		&User{},
	}

	for _, model := range models {
		t.Run(reflect.TypeOf(model).Elem().Name(), func(t *testing.T) {
			callBeforeCreate(t, model)

			id := reflect.ValueOf(model).Elem().FieldByName("ID").String()
			if len(id) != 25 || !strings.HasPrefix(id, "cl") {
				t.Fatalf("expected generated cuid-like ID, got %q", id)
			}
		})
	}
}

func TestBeforeCreatePreservesExistingID(t *testing.T) {
	user := &User{ID: "existing_user_id"}

	callBeforeCreate(t, user)

	if user.ID != "existing_user_id" {
		t.Fatalf("expected existing ID to be preserved, got %q", user.ID)
	}
}

func TestFailedEmbyAsyncOpBeforeCreateInitializesNextAttemptAt(t *testing.T) {
	op := &FailedEmbyAsyncOp{}

	callBeforeCreate(t, op)

	if op.NextAttemptAt.IsZero() {
		t.Fatalf("expected NextAttemptAt to be initialized")
	}
}

func TestPlaybackTransferTaskBeforeCreateInitializesAttempt(t *testing.T) {
	task := &PlaybackTransferTask{}

	callBeforeCreate(t, task)

	if task.Status != PlaybackTransferTaskStatusPending || task.AttemptCount != 1 || task.StartedAt.IsZero() {
		t.Fatalf("PlaybackTransferTask defaults = status=%s attempts=%d startedAt=%v",
			task.Status, task.AttemptCount, task.StartedAt)
	}
}

func TestEmbyAccessTokenJSONOmitsDigest(t *testing.T) {
	payload, err := json.Marshal(EmbyAccessToken{ID: "mapping-1", TokenHash: []byte("fixture-token-digest")})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(payload), "tokenHash") || strings.Contains(string(payload), "fixture-token-digest") {
		t.Fatalf("EmbyAccessToken JSON exposed digest: %s", payload)
	}
}

func TestUserBusinessHelpers(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	tests := []struct {
		name string
		user User
		want bool
	}{
		{name: "nil expiry", user: User{}, want: false},
		{name: "future expiry", user: User{ExpiresAt: &future}, want: false},
		{name: "past expiry", user: User{ExpiresAt: &past}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsExpired(); got != tt.want {
				t.Fatalf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}

	if !(&User{Role: "admin"}).IsAdmin() {
		t.Fatalf("expected admin role to be admin")
	}
	if (&User{Role: "user"}).IsAdmin() {
		t.Fatalf("expected user role not to be admin")
	}

	user := &User{}
	if err := user.SetPassword("correct-password"); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	if !user.CheckPassword("correct-password") {
		t.Fatalf("expected password to match")
	}
	if user.CheckPassword("wrong-password") {
		t.Fatalf("expected wrong password to be rejected")
	}
}

func TestExpiryHelpers(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	if !(&EmailVerification{ExpiresAt: past}).IsExpired() {
		t.Fatalf("expected past email verification to be expired")
	}
	if (&EmailVerification{ExpiresAt: future}).IsExpired() {
		t.Fatalf("expected future email verification to be active")
	}
	if !(&TelegramBindCode{ExpiresAt: past}).IsExpired() {
		t.Fatalf("expected past telegram bind code to be expired")
	}
	if (&TelegramBindCode{ExpiresAt: future}).IsExpired() {
		t.Fatalf("expected future telegram bind code to be active")
	}
}

func TestRedemptionCodeIsValid(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	tests := []struct {
		name string
		code RedemptionCode
		want bool
	}{
		{name: "usable without expiry", code: RedemptionCode{MaxUses: 2, UsedCount: 1}, want: true},
		{name: "usable before expiry", code: RedemptionCode{MaxUses: 2, UsedCount: 1, ExpiresAt: &future}, want: true},
		{name: "used up", code: RedemptionCode{MaxUses: 1, UsedCount: 1, ExpiresAt: &future}, want: false},
		{name: "expired", code: RedemptionCode{MaxUses: 2, UsedCount: 1, ExpiresAt: &past}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.code.IsValid(); got != tt.want {
				t.Fatalf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func callBeforeCreate(t *testing.T, model interface{}) {
	t.Helper()

	method := reflect.ValueOf(model).MethodByName("BeforeCreate")
	if !method.IsValid() {
		t.Fatalf("%T does not expose BeforeCreate", model)
	}
	results := method.Call([]reflect.Value{reflect.Zero(reflect.TypeOf(&gorm.DB{}))})
	if len(results) != 1 {
		t.Fatalf("expected one BeforeCreate return value, got %d", len(results))
	}
	if errValue := results[0]; !errValue.IsNil() {
		t.Fatalf("BeforeCreate() error = %v", errValue.Interface())
	}
}
