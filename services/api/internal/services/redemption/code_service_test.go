package redemption

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
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
			RegistrationPlanGroup: &planGroup,
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
			RegistrationPlanGroup: &planGroup,
		}, nil
	}

	service := &RedemptionCodeService{}
	code, err := service.ValidateRenewalCode("renew-code")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if code == nil || code.RegistrationPlanGroup == nil || *code.RegistrationPlanGroup != "VIP_A" {
		t.Fatalf("expected renewal validation to keep code payload, got %+v", code)
	}
}

func TestValidateRegistrationPlanGroupNormalizesAndAllowsEmpty(t *testing.T) {
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

	blank := "  "
	planGroup, err := service.validateRegistrationPlanGroup(&blank)
	if err != nil {
		t.Fatalf("expected blank plan group to be allowed, got %v", err)
	}
	if planGroup != nil {
		t.Fatalf("expected blank plan group to normalize to nil, got %+v", planGroup)
	}

	raw := "vip_a"
	planGroup, err = service.validateRegistrationPlanGroup(&raw)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if planGroup == nil || *planGroup != "VIP_A" {
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

	planGroup := "VIP_A"
	service := &RedemptionCodeService{}
	if err := service.ensureRegistrationPlanGroupExists(nil, &planGroup, true); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !lockCalled {
		t.Fatalf("expected locked lookup to be used")
	}
}
