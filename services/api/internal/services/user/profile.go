package user

import (
	"errors"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// UpdateProfileRequest 更新个人信息请求
type UpdateProfileRequest struct {
	Email string `json:"email" binding:"omitempty,email"`
}

// UpdateEmailRequest 修改邮箱请求
type UpdateEmailRequest struct {
	NewEmail string `json:"newEmail" binding:"required,email"`
}

func (s *UserService) GetProfile(userID string) (*UserView, error) {
	return s.GetUserByID(userID)
}

func (s *UserService) UpdateProfile(userID string, req *UpdateProfileRequest) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	if req.Email != "" {
		user.Email = req.Email
	}

	if err := db.DB.Save(&user).Error; err != nil {
		return nil, errors.New("更新失败")
	}

	return &user, nil
}

func (s *UserService) UpdateEmail(userID string, req *UpdateEmailRequest) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	user.Email = req.NewEmail

	if err := db.DB.Save(&user).Error; err != nil {
		return nil, errors.New("更新失败")
	}

	return &user, nil
}
