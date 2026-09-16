package user

import (
	"errors"
	"log"

	"gorm.io/gorm"
)

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// UpdatePasswordRequest 修改密码请求
type UpdatePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPassword 处理后台管理员发起的密码重置。
// 管理员账号只更新 Ember 本地密码；普通用户必须先同步 Emby 密码，再保存本地 hash。
func (s *UserService) ResetPassword(userID string, newPassword string) error {
	user, err := s.findUserByID(userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	if !user.IsAdmin() {
		if user.EmbyID == "" {
			log.Printf("ResetPassword: member missing emby id userId=%s", user.ID)
			return errors.New("重置密码失败：用户缺少 Emby ID")
		}

		embyService := s.embyClient()
		if err := embyService.UpdateUserPassword(user.EmbyID, newPassword); err != nil {
			log.Printf("ResetPassword: emby password update failed userId=%s embyId=%s errType=%T", user.ID, user.EmbyID, err)
			return errors.New("重置密码失败：" + err.Error())
		}
	} else {
		log.Printf("ResetPassword: updating local admin password userId=%s", user.ID)
	}

	if err := user.SetPassword(newPassword); err != nil {
		log.Printf("ResetPassword: local password hash failed userId=%s role=%s errType=%T", user.ID, user.Role, err)
		return errors.New("重置密码失败：本地密码更新失败")
	}
	user.PasswordResetRequired = false
	if err := s.saveUser(user); err != nil {
		log.Printf("ResetPassword: local password save failed userId=%s role=%s errType=%T", user.ID, user.Role, err)
		return errors.New("重置密码失败：本地密码保存失败")
	}

	return nil
}

func (s *UserService) UpdatePassword(userID string, req *UpdatePasswordRequest) error {
	user, err := s.findUserByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	if user.IsAdmin() {
		if !user.CheckPassword(req.OldPassword) {
			return ErrOldPasswordInvalid
		}
		if err := user.SetPassword(req.NewPassword); err != nil {
			return errors.New("密码更新失败：本地密码更新失败")
		}
		user.PasswordResetRequired = false
		if err := s.saveUser(user); err != nil {
			return errors.New("密码更新失败：本地密码保存失败")
		}
		return nil
	}

	embyService := s.newEmbyClient()
	oldPasswordVerified := false
	if _, err := embyService.AuthenticateUser(user.Username, req.OldPassword); err == nil {
		oldPasswordVerified = true
	}
	if !oldPasswordVerified && user.Password != "" && user.CheckPassword(req.OldPassword) {
		oldPasswordVerified = true
	}
	if !oldPasswordVerified {
		return ErrOldPasswordInvalid
	}

	if user.EmbyID == "" {
		return ErrUserEmbyIDRequired
	}
	if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
		return errors.New("密码更新失败：" + err.Error())
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return errors.New("密码更新失败：本地密码更新失败")
	}
	user.PasswordResetRequired = false
	if err := s.saveUser(user); err != nil {
		return errors.New("密码更新失败：本地密码保存失败")
	}

	return nil
}
