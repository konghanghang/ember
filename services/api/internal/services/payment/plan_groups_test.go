package payment

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

func TestUpdatePlanGroupSwitchesDefaultAndExpiresPendingPayments(t *testing.T) {
	origBegin := beginPlanGroupTx
	origCommit := commitPlanGroupTx
	origRollback := rollbackPlanGroupTx
	origGetByKeyForUpdate := paymentGetPlanGroupByKeyForUpdate
	origSave := paymentSavePlanGroup
	origUnsetOthers := paymentUnsetOtherPlanGroupDefaults
	origSetDefault := paymentSetPlanGroupDefault
	origExpireFollowing := paymentExpirePendingPaymentsForUsersFollowingDefault
	defer func() {
		beginPlanGroupTx = origBegin
		commitPlanGroupTx = origCommit
		rollbackPlanGroupTx = origRollback
		paymentGetPlanGroupByKeyForUpdate = origGetByKeyForUpdate
		paymentSavePlanGroup = origSave
		paymentUnsetOtherPlanGroupDefaults = origUnsetOthers
		paymentSetPlanGroupDefault = origSetDefault
		paymentExpirePendingPaymentsForUsersFollowingDefault = origExpireFollowing
	}()

	beginPlanGroupTx = func() (*gorm.DB, error) { return nil, nil }
	commitPlanGroupTx = func(tx *gorm.DB) error { return nil }
	rollbackPlanGroupTx = func(tx *gorm.DB) {}
	paymentGetPlanGroupByKeyForUpdate = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return &models.PlanGroup{Key: "VIP_B", Name: "VIP B", IsDefault: false}, nil
	}

	saved := false
	unsetOthersCalled := 0
	setDefaultCalled := 0
	expireCalled := 0

	paymentSavePlanGroup = func(tx *gorm.DB, group *models.PlanGroup) error {
		saved = true
		if group.Name != "升级组" {
			t.Fatalf("expected group name to be updated, got %q", group.Name)
		}
		return nil
	}
	paymentUnsetOtherPlanGroupDefaults = func(tx *gorm.DB, key string) error {
		unsetOthersCalled++
		if key != "VIP_B" {
			t.Fatalf("expected key VIP_B, got %s", key)
		}
		return nil
	}
	paymentSetPlanGroupDefault = func(tx *gorm.DB, key string, isDefault bool) error {
		setDefaultCalled++
		if key != "VIP_B" || !isDefault {
			t.Fatalf("unexpected default update: key=%s isDefault=%t", key, isDefault)
		}
		return nil
	}
	paymentExpirePendingPaymentsForUsersFollowingDefault = func(tx *gorm.DB) (int64, error) {
		expireCalled++
		return 2, nil
	}

	service := &PaymentService{}
	name := "升级组"
	isDefault := true
	updated, err := service.UpdatePlanGroup("vip_b", &UpdatePlanGroupRequest{
		Name:      &name,
		IsDefault: &isDefault,
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !saved {
		t.Fatalf("expected group to be saved")
	}
	if unsetOthersCalled != 1 || setDefaultCalled != 1 {
		t.Fatalf("expected default switch updates once, got unset=%d set=%d", unsetOthersCalled, setDefaultCalled)
	}
	if expireCalled != 1 {
		t.Fatalf("expected pending payments for default-following users to expire once, got %d", expireCalled)
	}
	if updated == nil || !updated.IsDefault {
		t.Fatalf("expected updated group to be default")
	}
}

func TestDeletePlanGroupRejectsReferencedGroup(t *testing.T) {
	origBegin := beginPlanGroupTx
	origCommit := commitPlanGroupTx
	origRollback := rollbackPlanGroupTx
	origGetByKeyForUpdate := paymentGetPlanGroupByKeyForUpdate
	origCountPlans := paymentCountPlansByGroup
	origCountUsers := paymentCountUsersByGroup
	origCountRedemptionCodes := paymentCountRedemptionCodesByRegistrationPlanGroup
	origDelete := paymentDeletePlanGroup
	defer func() {
		beginPlanGroupTx = origBegin
		commitPlanGroupTx = origCommit
		rollbackPlanGroupTx = origRollback
		paymentGetPlanGroupByKeyForUpdate = origGetByKeyForUpdate
		paymentCountPlansByGroup = origCountPlans
		paymentCountUsersByGroup = origCountUsers
		paymentCountRedemptionCodesByRegistrationPlanGroup = origCountRedemptionCodes
		paymentDeletePlanGroup = origDelete
	}()

	beginPlanGroupTx = func() (*gorm.DB, error) { return nil, nil }
	commitPlanGroupTx = func(tx *gorm.DB) error { return nil }
	rollbackPlanGroupTx = func(tx *gorm.DB) {}
	paymentGetPlanGroupByKeyForUpdate = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return &models.PlanGroup{Key: "VIP_B", Name: "VIP B", IsDefault: false}, nil
	}
	paymentCountPlansByGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 1, nil
	}
	paymentCountUsersByGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 0, nil
	}
	paymentCountRedemptionCodesByRegistrationPlanGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 0, nil
	}

	deleteCalled := false
	paymentDeletePlanGroup = func(tx *gorm.DB, key string) error {
		deleteCalled = true
		return nil
	}

	service := &PaymentService{}
	err := service.DeletePlanGroup("vip_b")
	if !errors.Is(err, ErrPlanGroupDeleteBlocked) {
		t.Fatalf("expected ErrPlanGroupDeleteBlocked, got %v", err)
	}
	if deleteCalled {
		t.Fatalf("expected referenced group delete to stop before delete call")
	}
}

func TestDeletePlanGroupRejectsRedemptionCodeReference(t *testing.T) {
	origBegin := beginPlanGroupTx
	origCommit := commitPlanGroupTx
	origRollback := rollbackPlanGroupTx
	origGetByKeyForUpdate := paymentGetPlanGroupByKeyForUpdate
	origCountPlans := paymentCountPlansByGroup
	origCountUsers := paymentCountUsersByGroup
	origCountRedemptionCodes := paymentCountRedemptionCodesByRegistrationPlanGroup
	origDelete := paymentDeletePlanGroup
	defer func() {
		beginPlanGroupTx = origBegin
		commitPlanGroupTx = origCommit
		rollbackPlanGroupTx = origRollback
		paymentGetPlanGroupByKeyForUpdate = origGetByKeyForUpdate
		paymentCountPlansByGroup = origCountPlans
		paymentCountUsersByGroup = origCountUsers
		paymentCountRedemptionCodesByRegistrationPlanGroup = origCountRedemptionCodes
		paymentDeletePlanGroup = origDelete
	}()

	beginPlanGroupTx = func() (*gorm.DB, error) { return nil, nil }
	commitPlanGroupTx = func(tx *gorm.DB) error { return nil }
	rollbackPlanGroupTx = func(tx *gorm.DB) {}
	paymentGetPlanGroupByKeyForUpdate = func(tx *gorm.DB, key string) (*models.PlanGroup, error) {
		return &models.PlanGroup{Key: "VIP_B", Name: "VIP B", IsDefault: false}, nil
	}
	paymentCountPlansByGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 0, nil
	}
	paymentCountUsersByGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 0, nil
	}
	paymentCountRedemptionCodesByRegistrationPlanGroup = func(tx *gorm.DB, key string) (int64, error) {
		return 1, nil
	}

	deleteCalled := false
	paymentDeletePlanGroup = func(tx *gorm.DB, key string) error {
		deleteCalled = true
		return nil
	}

	service := &PaymentService{}
	err := service.DeletePlanGroup("vip_b")
	if !errors.Is(err, ErrPlanGroupDeleteBlocked) {
		t.Fatalf("expected ErrPlanGroupDeleteBlocked, got %v", err)
	}
	if deleteCalled {
		t.Fatalf("expected redemption-code-referenced group delete to stop before delete call")
	}
}
