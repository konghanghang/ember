package user

import (
	"errors"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// ResetPasswordByCodeRequest 通过邮箱验证码重置密码
type ResetPasswordByCodeRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

func (s *UserService) ResetPasswordByCode(req *ResetPasswordByCodeRequest) error {
	s.setDefaults()

	if db.DB == nil {
		return s.resetPasswordByCodeCompat(req)
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	if err := s.getEmailVerifier().ConsumeCodeTx(tx, req.Email, req.Code, models.VerificationTypeReset); err != nil {
		tx.Rollback()
		return err
	}

	var user models.User
	if err := tx.Where("lower(email) = ?", strings.ToLower(strings.TrimSpace(req.Email))).First(&user).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return ErrUserNotFound
	}

	if user.EmbyID != "" {
		embyService := s.newEmbyClient()
		if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
			tx.Rollback()
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		tx.Rollback()
		return errors.New("密码重置失败：本地密码更新失败")
	}
	user.PasswordResetRequired = false
	if err := tx.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]interface{}{
			"password":              user.Password,
			"passwordResetRequired": false,
		}).Error; err != nil {
		tx.Rollback()
		return errors.New("密码重置失败：本地密码保存失败")
	}

	if err := tx.Commit().Error; err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	return nil
}

func (s *UserService) resetPasswordByCodeCompat(req *ResetPasswordByCodeRequest) error {
	if err := s.getEmailVerifier().VerifyCode(req.Email, req.Code, models.VerificationTypeReset); err != nil {
		return err
	}

	user, err := s.findUserByEmail(req.Email)
	if err != nil {
		return ErrUserNotFound
	}

	if user.EmbyID != "" {
		embyService := s.newEmbyClient()
		if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
			return errors.New("密码重置失败：" + err.Error())
		}
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return errors.New("密码重置失败：本地密码更新失败")
	}
	user.PasswordResetRequired = false
	if err := s.saveUser(user); err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	return nil
}
