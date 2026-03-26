package services

import (
	"errors"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

// ResetPasswordByCodeRequest 通过邮箱验证码重置密码
type ResetPasswordByCodeRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPasswordByCode 通过邮箱验证码重置密码
func (s *UserService) ResetPasswordByCode(req *ResetPasswordByCodeRequest) error {
	if err := s.getEmailVerifier().VerifyCode(req.Email, req.Code, models.VerificationTypeReset); err != nil {
		return err
	}

	var user models.User
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return errors.New("用户不存在")
	}

	if user.EmbyID != "" {
		embyService := embyint.NewEmbyService()
		if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return errors.New("密码重置失败：本地密码更新失败")
	}
	tx := db.DB.Begin()
	if tx.Error != nil {
		return errors.New("密码重置失败：系统繁忙，请稍后重试")
	}

	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		return errors.New("密码重置失败：本地密码保存失败")
	}

	deleteResult := tx.Where("email = ? AND \"type\" = ?", req.Email, models.VerificationTypeReset).
		Delete(&models.EmailVerification{})
	if deleteResult.Error != nil {
		tx.Rollback()
		return errors.New("密码重置失败：验证码清理失败")
	}
	if deleteResult.RowsAffected == 0 {
		tx.Rollback()
		return emailpkg.ErrEmailCodeInvalid
	}
	if err := tx.Commit().Error; err != nil {
		return errors.New("密码重置失败：验证码清理失败")
	}

	return nil
}
