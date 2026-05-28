package auth

import (
	"errors"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
	"gorm.io/gorm"
)

func (s *AuthService) persistRegisteredUser(
	req *RegisterUserRequest,
	prepared *registerPreparation,
	embyUser *embyint.EmbyUser,
) (*models.User, error) {
	user, err := s.buildRegisteredUser(req, prepared, embyUser)
	if err != nil {
		return nil, err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("创建用户失败")
	}

	if s.emailService.IsEnabled() {
		if err := s.emailService.ConsumeCodeTx(tx, req.Email, req.EmailCode, models.VerificationTypeRegister); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, errors.New("创建用户失败")
	}

	if err := s.applyInviteRegistration(tx, req, prepared, user); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("创建用户失败")
	}
	if err := policypkg.NewService(s.newEmbyClient()).ApplyEffectiveUserPolicy(user.ID, "user_registered"); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) buildRegisteredUser(
	req *RegisterUserRequest,
	prepared *registerPreparation,
	embyUser *embyint.EmbyUser,
) (*models.User, error) {
	expiresAt := common.CalculateExpiryDate(prepared.defaultDays)

	user := &models.User{
		Username:  req.Username,
		Role:      "user",
		Email:     req.Email,
		EmbyID:    embyUser.ID,
		ExpiresAt: &expiresAt,
		IsActive:  true,
	}
	planGroup, err := s.resolveRegistrationPlanGroup(prepared)
	if err != nil {
		return nil, err
	}
	user.PlanGroup = &planGroup
	if err := user.SetPassword(req.Password); err != nil {
		return nil, errors.New("创建用户失败")
	}

	return user, nil
}

// resolveRegistrationPlanGroup 为注册用户解析必须持久化的套餐分组。
// 邀请码显式绑定分组时优先使用该分组；开放注册或旧邀请码未绑定时写入当前默认分组，
// 避免后续模板同步链路再依赖 users.plan_group IS NULL 的隐式跟随语义。
func (s *AuthService) resolveRegistrationPlanGroup(prepared *registerPreparation) (string, error) {
	if prepared != nil && prepared.registrationPlanGroup != nil {
		return *prepared.registrationPlanGroup, nil
	}
	if s.getDefaultPlanGroup == nil {
		return "", errors.New("默认套餐分组不可用")
	}
	group, err := s.getDefaultPlanGroup()
	if err != nil {
		return "", err
	}
	return group.Key, nil
}

func (s *AuthService) applyInviteRegistration(
	tx *gorm.DB,
	req *RegisterUserRequest,
	prepared *registerPreparation,
	user *models.User,
) error {
	if prepared.mode != "invite" || prepared.redemptionCode == nil {
		return nil
	}

	result := tx.Model(&models.RedemptionCode{}).
		Where("code = ? AND \"used_count\" < \"max_uses\"", strings.TrimSpace(req.Code)).
		Update("used_count", gorm.Expr("\"used_count\" + 1"))
	if result.Error != nil {
		return errors.New("创建用户失败")
	}
	if result.RowsAffected == 0 {
		return redemptionpkg.ErrRedemptionCodeInvalid
	}

	redemption := models.Redemption{
		UserID: user.ID,
		Code:   strings.TrimSpace(req.Code),
		Days:   prepared.defaultDays,
	}
	if err := tx.Create(&redemption).Error; err != nil {
		return errors.New("创建用户失败")
	}

	return nil
}
