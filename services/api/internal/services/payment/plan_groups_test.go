package payment

import (
	"errors"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
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

func TestPlanGroupManagedPolicyTaskQueryExcludesAdminsAndProtectionFailures(t *testing.T) {
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

	var count int64
	stmt := planGroupManagedPolicyTaskQuery(database, "VIP_A").
		Where("tasks.status = ?", "failed").
		Count(&count).Statement
	sql := strings.Join(strings.Fields(stmt.SQL.String()), " ")

	assertSQLContains(t, sql, "JOIN users ON users.id = tasks.user_id")
	assertSQLContains(t, sql, "tasks.plan_group_key =")
	assertSQLContains(t, sql, "users.role =")
	assertSQLContains(t, sql, "NOT (tasks.status =")
	assertSQLContains(t, sql, "COALESCE(tasks.last_error, '') LIKE")
	if len(stmt.Vars) != 5 || stmt.Vars[0] != "VIP_A" || stmt.Vars[1] != "user" || stmt.Vars[2] != "failed" || stmt.Vars[4] != "failed" {
		t.Fatalf("unexpected query vars: %+v", stmt.Vars)
	}
	pattern, ok := stmt.Vars[3].(string)
	if !ok || !strings.Contains(pattern, "There must be at least one user in the system with administrative access") {
		t.Fatalf("expected admin protection pattern, got %+v", stmt.Vars[3])
	}
}

func TestBuildPlansWithGroupNameSelectIncludesDisplayJoin(t *testing.T) {
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

	var plans []PlanView
	stmt := buildPlansWithGroupNameSelect(database.Model(&models.Plan{})).
		Where(`plans."is_active" = ?`, true).
		Find(&plans).Statement
	sql := strings.Join(strings.Fields(stmt.SQL.String()), " ")

	assertSQLContains(t, sql, `plans.*, plan_groups.name AS "planGroupName"`)
	assertSQLContains(t, sql, `LEFT JOIN plan_groups ON plan_groups.key = plans."plan_group"`)
	assertSQLContains(t, sql, `plans."is_active" =`)
	if len(stmt.Vars) != 1 || stmt.Vars[0] != true {
		t.Fatalf("expected active filter var, got %+v", stmt.Vars)
	}
}

func TestBuildPlanGroupViewKeepsPersistentFields(t *testing.T) {
	group := models.PlanGroup{
		Key:       "VIP_A",
		Name:      "VIP A",
		IsDefault: true,
	}

	view := buildPlanGroupView(group)
	if view.Key != "VIP_A" || view.Name != "VIP A" || !view.IsDefault {
		t.Fatalf("expected plan group fields to be preserved, got %+v", view)
	}
	if view.PlanCount != 0 || view.UserCount != 0 || view.PolicySyncStatus != "" {
		t.Fatalf("expected aggregate fields to start empty, got %+v", view)
	}
}

func TestResolveEffectivePlanGroupKeyReturnsExplicitKeyWhenDBUnavailable(t *testing.T) {
	explicit := " vip-b "

	got, err := ResolveEffectivePlanGroupKey(nil, &explicit)
	if err != nil {
		t.Fatalf("ResolveEffectivePlanGroupKey() error = %v", err)
	}
	if got != "VIP-B" {
		t.Fatalf("ResolveEffectivePlanGroupKey() = %q, want VIP-B", got)
	}
}

func TestResolveEffectivePlanGroupKeyRejectsInvalidExplicitKeyBeforeDB(t *testing.T) {
	explicit := "vip b"

	_, err := ResolveEffectivePlanGroupKey(nil, &explicit)
	if !errors.Is(err, ErrPlanGroupInvalid) {
		t.Fatalf("expected ErrPlanGroupInvalid, got %v", err)
	}
}

func assertSQLContains(t *testing.T, sql string, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL to contain %q, got %s", fragment, sql)
	}
}
