package user

import (
	"errors"

	"github.com/konghang/ember/backend/internal/models"
)

// ResetPasswordByCodeRequest 通过邮箱验证码重置密码
type ResetPasswordByCodeRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

func (s *UserService) ResetPasswordByCode(req *ResetPasswordByCodeRequest) error {
	s.setDefaults()

	if err := s.getEmailVerifier().VerifyCode(req.Email, req.Code, models.VerificationTypeReset); err != nil {
		return err
	}

	user, err := s.findUserByEmail(req.Email)
	if err != nil {
		return errors.New("用户不存在")
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
	if err := s.saveUser(user); err != nil {
		return errors.New("密码重置失败：本地密码保存失败")
	}

	return nil
}
