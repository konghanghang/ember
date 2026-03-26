package user

import (
	"errors"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
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
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	embyService := embyint.NewEmbyService()
	err := embyService.UpdateUserPassword(user.EmbyID, newPassword)
	if err != nil {
		return errors.New("重置密码失败：" + err.Error())
	}

	if err := user.SetPassword(newPassword); err != nil {
		return errors.New("重置密码失败：本地密码更新失败")
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return errors.New("重置密码失败：本地密码保存失败")
	}

	return nil
}

func (s *UserService) UpdatePassword(userID string, req *UpdatePasswordRequest) error {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	if user.IsAdmin() {
		if !user.CheckPassword(req.OldPassword) {
			return errors.New("旧密码错误")
		}
		if err := user.SetPassword(req.NewPassword); err != nil {
			return errors.New("密码更新失败：本地密码更新失败")
		}
		if err := db.DB.Save(&user).Error; err != nil {
			return errors.New("密码更新失败：本地密码保存失败")
		}
		return nil
	}

	embyService := embyint.NewEmbyService()
	oldPasswordVerified := false
	if _, err := embyService.AuthenticateUser(user.Username, req.OldPassword); err == nil {
		oldPasswordVerified = true
	}
	if !oldPasswordVerified && user.Password != "" && user.CheckPassword(req.OldPassword) {
		oldPasswordVerified = true
	}
	if !oldPasswordVerified {
		return errors.New("旧密码错误")
	}

	if user.EmbyID == "" {
		return errors.New("密码更新失败：用户缺少 Emby ID")
	}
	if err := embyService.UpdateUserPassword(user.EmbyID, req.NewPassword); err != nil {
		return errors.New("密码更新失败：" + err.Error())
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		return errors.New("密码更新失败：本地密码更新失败")
	}
	if err := db.DB.Save(&user).Error; err != nil {
		return errors.New("密码更新失败：本地密码保存失败")
	}

	return nil
}
