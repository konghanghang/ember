package services

import (
	"errors"
	"log"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

type AuthService struct{}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	var user models.User
	result := db.DB.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户名或密码错误")
	}

	if user.IsAdmin() {
		if !user.CheckPassword(req.Password) {
			return nil, errors.New("用户名或密码错误")
		}
	} else {
		if !user.IsActive {
			return nil, errors.New("账号已被禁用")
		}
		if user.IsExpired() {
			return nil, errors.New("账号已过期")
		}
		embyService := NewEmbyService()
		embyUser, err := embyService.AuthenticateUser(user.Username, req.Password)
		if err != nil {
			return nil, errors.New("用户名或密码错误")
		}
		if embyUser.ID != user.EmbyID {
			return nil, errors.New("用户信息不匹配")
		}
	}

	token, err := common.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &LoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

type RegisterUserRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=50"`
	Password   string `json:"password" binding:"required,min=6"`
	Email      string `json:"email" binding:"required,email"`
	InviteCode string `json:"inviteCode" binding:"required"`
}

type RegisterUserResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	inviteService := &InviteService{}
	invite, err := inviteService.ValidateInvite(req.InviteCode)
	if err != nil {
		return nil, err
	}

	var existingUser models.User
	result := db.DB.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		return nil, errors.New("用户名已存在")
	}

	embyService := NewEmbyService()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	expiresAt := common.CalculateExpiryDate(invite.DefaultDays)

	user := models.User{
		Username:   req.Username,
		Role:       "user",
		Email:      req.Email,
		EmbyID:     embyUser.ID,
		InviteCode: req.InviteCode,
		ExpiresAt:  &expiresAt,
		IsActive:   true,
	}

	if err := db.DB.Create(&user).Error; err != nil {
		return nil, errors.New("创建用户失败")
	}

	if err := inviteService.UseInvite(req.InviteCode); err != nil {
		log.Printf("⚠️  邀请码使用次数更新失败（不影响注册）：code=%s, err=%v", req.InviteCode, err)
	}

	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
		Token: token,
		User:  &user,
	}, nil
}

func (s *AuthService) GetCurrentUser(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}
