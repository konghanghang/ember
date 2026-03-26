package services

import (
	"errors"

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
	user, err := s.buildRegisteredUser(req, prepared.defaultDays, embyUser)
	if err != nil {
		return nil, err
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, errors.New("创建用户失败")
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
	defaultDays int,
	embyUser *embyint.EmbyUser,
) (*models.User, error) {
	expiresAt := common.CalculateExpiryDate(defaultDays)

	user := &models.User{
		Username:  req.Username,
		Role:      "user",
		Email:     req.Email,
		EmbyID:    embyUser.ID,
		ExpiresAt: &expiresAt,
		IsActive:  true,
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
		Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
		Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
	if result.Error != nil {
		return errors.New("创建用户失败")
	}
	if result.RowsAffected == 0 {
		return redemptionpkg.ErrRedemptionCodeInvalid
	}

	redemption := models.Redemption{
		UserID: user.ID,
		Code:   req.Code,
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
