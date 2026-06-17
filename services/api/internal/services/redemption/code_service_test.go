package redemption

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/db"
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

func TestCreateRedemptionCodesRejectsInvalidBatchCountBeforeValidation(t *testing.T) {
	origNormalize := redemptionNormalizePlanGroupKey
	defer func() {
		redemptionNormalizePlanGroupKey = origNormalize
	}()
	redemptionNormalizePlanGroupKey = func(raw string, allowEmpty bool) (string, error) {
		t.Fatalf("plan group validation must not run for invalid batch count")
		return "", nil
	}

	service := &RedemptionCodeService{}
	for _, count := range []int{0, maxCreateRedemptionCodesCount + 1} {
		t.Run("count", func(t *testing.T) {
			_, err := service.createRedemptionCodes(RedemptionCodeCreateOptions{
				MaxUses:               1,
				DefaultDays:           30,
				RegistrationPlanGroup: "VIP_A",
			}, count)
			if !errors.Is(err, ErrRedemptionCodeBatchCountInvalid) {
				t.Fatalf("expected ErrRedemptionCodeBatchCountInvalid, got %v", err)
			}
		})
	}
}

func TestGenerateCodeReturnsRequestedLowerHexLength(t *testing.T) {
	service := &RedemptionCodeService{}
	hexPattern := regexp.MustCompile(`^[0-9a-f]+$`)

	for _, length := range []int{1, 7, 16, 20} {
		t.Run("length", func(t *testing.T) {
			code, err := service.generateCode(length)
			if err != nil {
				t.Fatalf("generate code: %v", err)
			}
			if len(code) != length {
				t.Fatalf("expected length %d, got %d (%q)", length, len(code), code)
			}
			if !hexPattern.MatchString(code) {
				t.Fatalf("expected lower hex code, got %q", code)
			}
		})
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

func TestEnsureRegistrationPlanGroupAvailableMapsMissingAndPropagatesLookupErrors(t *testing.T) {
	origGetPlanGroup := redemptionGetPlanGroupByKey
	defer func() {
		redemptionGetPlanGroupByKey = origGetPlanGroup
	}()

	service := &RedemptionCodeService{}
	if err := service.ensureRegistrationPlanGroupAvailable(nil); !errors.Is(err, ErrRegistrationPlanGroupRequired) {
		t.Fatalf("expected ErrRegistrationPlanGroupRequired for nil code, got %v", err)
	}
	if err := service.ensureRegistrationPlanGroupAvailable(&models.RedemptionCode{}); !errors.Is(err, ErrRegistrationPlanGroupRequired) {
		t.Fatalf("expected ErrRegistrationPlanGroupRequired for blank plan group, got %v", err)
	}

	redemptionGetPlanGroupByKey = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		if key != "VIP_A" {
			t.Fatalf("expected lookup key VIP_A, got %q", key)
		}
		return &models.PlanGroup{Key: key, Name: "VIP A"}, nil
	}
	if err := service.ensureRegistrationPlanGroupAvailable(&models.RedemptionCode{RegistrationPlanGroup: "VIP_A"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	redemptionGetPlanGroupByKey = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return nil, paymentpkg.ErrPlanGroupNotFound
	}
	if err := service.ensureRegistrationPlanGroupAvailable(&models.RedemptionCode{ID: "rcode_1", RegistrationPlanGroup: "VIP_A"}); !errors.Is(err, ErrRegistrationPlanGroupNotFound) {
		t.Fatalf("expected ErrRegistrationPlanGroupNotFound, got %v", err)
	}

	lookupErr := errors.New("lookup failed")
	redemptionGetPlanGroupByKey = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return nil, lookupErr
	}
	if err := service.ensureRegistrationPlanGroupAvailable(&models.RedemptionCode{RegistrationPlanGroup: "VIP_A"}); !errors.Is(err, lookupErr) {
		t.Fatalf("expected original lookup error, got %v", err)
	}
}

func TestEnrichCodeHandlesNilAndSkipsDisplayLookupWithoutDB(t *testing.T) {
	originalDB := db.DB
	db.DB = nil
	defer func() {
		db.DB = originalDB
	}()

	service := &RedemptionCodeService{}
	if err := service.enrichCode(nil); err != nil {
		t.Fatalf("expected nil code to be ignored, got %v", err)
	}

	code := &models.RedemptionCode{
		ID:                    "rcode_1",
		Code:                  "invite-code",
		RegistrationPlanGroup: "VIP_A",
	}
	if err := service.enrichCode(code); err != nil {
		t.Fatalf("expected enrich without DB to succeed, got %v", err)
	}
	if code.RegistrationPlanGroupName != nil {
		t.Fatalf("expected no display name without DB, got %+v", code.RegistrationPlanGroupName)
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

func TestIsRedemptionDuplicateInsertDetectsPostgresDuplicateKey(t *testing.T) {
	if !isRedemptionDuplicateInsert(&pgconn.PgError{Code: "23505"}) {
		t.Fatalf("expected duplicate key pg error to be a redemption duplicate insert")
	}
	if isRedemptionDuplicateInsert(&pgconn.PgError{Code: "23503"}) {
		t.Fatalf("expected non-duplicate pg error to be ignored")
	}
	if isRedemptionDuplicateInsert(errors.New("plain error")) {
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

func TestBuildUsableCodeConsumptionQueryGuardsUsageAndExpiry(t *testing.T) {
	database := newRedemptionDryRunDB(t)
	now := time.Date(2026, 6, 17, 8, 30, 0, 0, time.UTC)

	var codes []models.RedemptionCode
	stmt := buildUsableCodeConsumptionQuery(database, "  renew-code  ", now).Find(&codes).Statement
	sql := normalizeRedemptionSQL(stmt.SQL.String())

	assertRedemptionSQLContains(t, sql, `code =`)
	assertRedemptionSQLContains(t, sql, `"used_count" < "max_uses"`)
	assertRedemptionSQLContains(t, sql, `"expires_at" IS NULL OR "expires_at" >`)
	if len(stmt.Vars) != 2 {
		t.Fatalf("expected code and now vars, got %+v", stmt.Vars)
	}
	if stmt.Vars[0] != "renew-code" {
		t.Fatalf("expected trimmed code var, got %+v", stmt.Vars[0])
	}
	if !stmt.Vars[1].(time.Time).Equal(now) {
		t.Fatalf("expected now var %s, got %+v", now, stmt.Vars[1])
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
