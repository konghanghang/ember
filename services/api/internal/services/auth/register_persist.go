package auth

import (
	"errors"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
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

	if err := s.applyInviteRegistration(tx, req, prepared, user, embyUser.ID); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, errors.New("创建用户失败")
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
	if prepared.registrationPlanGroup != nil {
		planGroup := *prepared.registrationPlanGroup
		user.PlanGroup = &planGroup
	}
	if err := user.SetPassword(req.Password); err != nil {
		return nil, errors.New("创建用户失败")
	}

	return user, nil
}

func (s *AuthService) applyInviteRegistration(
	tx *gorm.DB,
	req *RegisterUserRequest,
	prepared *registerPreparation,
	user *models.User,
	embyUserID string,
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

	if err := s.applyTemplatePolicyIfNeeded(embyUserID, prepared.redemptionCode.TemplateUserID); err != nil {
		return err
	}

	return nil
}
