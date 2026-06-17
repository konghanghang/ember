package redemption

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestValidateRegistrationCodeRejectsMissingBoundPlanGroup(t *testing.T) {
	origFindCode := redemptionFindCodeByValue
	origGetPlanGroup := redemptionGetPlanGroupByKey
	defer func() {
		redemptionFindCodeByValue = origFindCode
		redemptionGetPlanGroupByKey = origGetPlanGroup
	}()

	planGroup := "VIP_A"
	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		return &models.RedemptionCode{
			ID:                    "rcode_1",
			Code:                  code,
			MaxUses:               3,
			UsedCount:             0,
			DefaultDays:           30,
			RegistrationPlanGroup: planGroup,
		}, nil
	}
	redemptionGetPlanGroupByKey = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return nil, paymentpkg.ErrPlanGroupNotFound
	}

	service := &RedemptionCodeService{}
	_, err := service.ValidateRegistrationCode("invite-code")
	if !errors.Is(err, ErrRegistrationPlanGroupNotFound) {
		t.Fatalf("expected ErrRegistrationPlanGroupNotFound, got %v", err)
	}
}

func TestValidateRenewalCodeIgnoresMissingBoundPlanGroup(t *testing.T) {
	origFindCode := redemptionFindCodeByValue
	defer func() {
		redemptionFindCodeByValue = origFindCode
	}()

	planGroup := "VIP_A"
	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		return &models.RedemptionCode{
			ID:                    "rcode_2",
			Code:                  code,
			MaxUses:               3,
			UsedCount:             0,
			DefaultDays:           30,
			RegistrationPlanGroup: planGroup,
		}, nil
	}

	service := &RedemptionCodeService{}
	code, err := service.ValidateRenewalCode("renew-code")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if code == nil || code.RegistrationPlanGroup != "VIP_A" {
		t.Fatalf("expected renewal validation to keep code payload, got %+v", code)
	}
}

func TestValidateRegistrationPlanGroupRequiresExplicitValue(t *testing.T) {
	origNormalize := redemptionNormalizePlanGroupKey
	defer func() {
		redemptionNormalizePlanGroupKey = origNormalize
	}()

	service := &RedemptionCodeService{}
	redemptionNormalizePlanGroupKey = func(raw string, allowEmpty bool) (string, error) {
		if raw == "  " && allowEmpty {
			return "", nil
		}
		return "VIP_A", nil
	}

	_, err := service.validateRegistrationPlanGroup("  ")
	if !errors.Is(err, ErrRegistrationPlanGroupRequired) {
		t.Fatalf("expected ErrRegistrationPlanGroupRequired, got %v", err)
	}

	planGroup, err := service.validateRegistrationPlanGroup("vip_a")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if planGroup != "VIP_A" {
		t.Fatalf("expected normalized plan group VIP_A, got %+v", planGroup)
	}
}

func TestEnsureRegistrationPlanGroupExistsUsesLockedLookup(t *testing.T) {
	origGetPlanGroup := redemptionGetPlanGroupByKey
	origGetPlanGroupForUpdate := redemptionGetPlanGroupForUpdate
	defer func() {
		redemptionGetPlanGroupByKey = origGetPlanGroup
		redemptionGetPlanGroupForUpdate = origGetPlanGroupForUpdate
	}()

	lockCalled := false
	redemptionGetPlanGroupByKey = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		t.Fatalf("unexpected unlocked lookup for key %s", key)
		return nil, nil
	}
	redemptionGetPlanGroupForUpdate = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		lockCalled = true
		return &models.PlanGroup{Key: key, Name: "VIP A"}, nil
	}

	service := &RedemptionCodeService{}
	if err := service.ensureRegistrationPlanGroupExists(nil, "VIP_A", true); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !lockCalled {
		t.Fatalf("expected locked lookup to be used")
	}
}

func TestValidateUsableCodeTrimsInputAndMapsLookupErrors(t *testing.T) {
	origFindCode := redemptionFindCodeByValue
	defer func() {
		redemptionFindCodeByValue = origFindCode
	}()

	service := &RedemptionCodeService{}
	var lookupCode string
	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		lookupCode = code
		return &models.RedemptionCode{
			ID:          "rcode_usable",
			Code:        code,
			MaxUses:     2,
			UsedCount:   1,
			DefaultDays: 30,
		}, nil
	}

	code, err := service.validateUsableCode("  invite-code  ")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if lookupCode != "invite-code" {
		t.Fatalf("expected lookup to use trimmed code, got %q", lookupCode)
	}
	if code == nil || code.Code != "invite-code" {
		t.Fatalf("expected validated code payload, got %+v", code)
	}

	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		return nil, gorm.ErrRecordNotFound
	}
	if _, err := service.validateUsableCode("missing-code"); !errors.Is(err, ErrRedemptionCodeNotFound) {
		t.Fatalf("expected ErrRedemptionCodeNotFound, got %v", err)
	}

	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		return nil, errors.New("database unavailable")
	}
	if _, err := service.validateUsableCode("broken-code"); err == nil || err.Error() != "校验兑换码失败" {
		t.Fatalf("expected masked lookup error, got %v", err)
	}

	redemptionFindCodeByValue = func(code string) (*models.RedemptionCode, error) {
		return &models.RedemptionCode{
			ID:        "rcode_exhausted",
			Code:      code,
			MaxUses:   1,
			UsedCount: 1,
		}, nil
	}
	if _, err := service.validateUsableCode("exhausted-code"); !errors.Is(err, ErrRedemptionCodeInvalid) {
		t.Fatalf("expected ErrRedemptionCodeInvalid, got %v", err)
	}
}

func TestIsRedemptionCodeConflictDetectsPostgresDuplicateKey(t *testing.T) {
	if !isRedemptionCodeConflict(&pgconn.PgError{Code: "23505"}) {
		t.Fatalf("expected duplicate key pg error to be a redemption code conflict")
	}
	if isRedemptionCodeConflict(&pgconn.PgError{Code: "23503"}) {
		t.Fatalf("expected non-duplicate pg error to be ignored")
	}
	if isRedemptionCodeConflict(errors.New("plain error")) {
		t.Fatalf("expected non-pg error to be ignored")
	}
}

func TestApplyRedemptionCodeStatusFilterBuildsExpectedPredicates(t *testing.T) {
	database := newRedemptionDryRunDB(t)
	now := time.Date(2026, 6, 17, 8, 30, 0, 0, time.UTC)

	tests := []struct {
		name          string
		status        RedemptionCodeStatus
		wantFragments []string
		wantVars      int
	}{
		{
			name:          "active",
			status:        RedemptionCodeStatusActive,
			wantFragments: []string{`"used_count" < "max_uses"`, `"expires_at" IS NULL OR "expires_at" >`},
			wantVars:      1,
		},
		{
			name:          "expired",
			status:        RedemptionCodeStatusExpired,
			wantFragments: []string{`"used_count" < "max_uses"`, `"expires_at" IS NOT NULL`, `"expires_at" <=`},
			wantVars:      1,
		},
		{
			name:          "exhausted",
			status:        RedemptionCodeStatusExhausted,
			wantFragments: []string{`"used_count" >= "max_uses"`},
			wantVars:      0,
		},
		{
			name:          "empty",
			status:        "",
			wantFragments: nil,
			wantVars:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := applyRedemptionCodeStatusFilter(database.Model(&models.RedemptionCode{}), tt.status, now)
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}

			var codes []models.RedemptionCode
			stmt := query.Find(&codes).Statement
			sql := normalizeRedemptionSQL(stmt.SQL.String())

			for _, fragment := range tt.wantFragments {
				assertRedemptionSQLContains(t, sql, fragment)
			}
			if len(stmt.Vars) != tt.wantVars {
				t.Fatalf("expected %d vars, got %+v", tt.wantVars, stmt.Vars)
			}
			if tt.wantVars == 1 && !stmt.Vars[0].(time.Time).Equal(now) {
				t.Fatalf("expected now var %s, got %+v", now, stmt.Vars)
			}
			if tt.status == "" && strings.Contains(sql, " WHERE ") {
				t.Fatalf("expected empty status to keep query unfiltered, got %s", sql)
			}
		})
	}

	if _, err := applyRedemptionCodeStatusFilter(database.Model(&models.RedemptionCode{}), RedemptionCodeStatus("bad"), now); !errors.Is(err, ErrRedemptionCodeStatusInvalid) {
		t.Fatalf("expected ErrRedemptionCodeStatusInvalid, got %v", err)
	}
}

func newRedemptionDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	return database
}

func normalizeRedemptionSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func assertRedemptionSQLContains(t *testing.T, sql string, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL to contain %q, got %s", fragment, sql)
	}
}
