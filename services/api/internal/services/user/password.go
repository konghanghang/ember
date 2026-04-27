package user

import (
	"errors"
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

func (s *UserService) ResetPassword(userID string, newPassword string) error {
	user, err := s.findUserByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	embyService := s.newEmbyClient()
	err = embyService.UpdateUserPassword(user.EmbyID, newPassword)
	if err != nil {
		return errors.New("重置密码失败：" + err.Error())
	}

	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("重置密码失败：本地密码更新失败")
	}
	user.PasswordResetRequired = false
	if err := s.saveUser(user); err != nil {
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
