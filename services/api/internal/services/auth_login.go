package services

import (
	"errors"
	"log"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// LoginRequest 统一登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 统一登录响应
type LoginResponse struct {
	Token     string       `json:"token"`
	User      *models.User `json:"user"`
	IsExpired bool         `json:"isExpired"`
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	user, err := s.findLoginUser(req.Username)
	if err != nil {
		return nil, err
	}

	if err := s.authenticateLoginUser(user, req.Password); err != nil {
		return nil, err
	}

	token, err := common.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &LoginResponse{
		Token:     token,
		User:      user,
		IsExpired: user.IsExpired(),
	}, nil
}

func (s *AuthService) findLoginUser(username string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户名或密码错误")
	}
	return &user, nil
}

func (s *AuthService) authenticateLoginUser(user *models.User, password string) error {
	if user.IsAdmin() {
		if user.CheckPassword(password) {
			return nil
		}
		return errors.New("用户名或密码错误")
	}

	embyService := s.newEmbyClient()
	embyUser, err := embyService.AuthenticateUser(user.Username, password)
	if err == nil && embyUser.ID == user.EmbyID {
		s.syncLocalLoginHash(user, password)
		return nil
	}

	if user.Password != "" && user.CheckPassword(password) {
		if user.EmbyID != "" {
			if syncErr := embyService.UpdateUserPassword(user.EmbyID, password); syncErr != nil {
				log.Printf("⚠️  登录时同步 Emby 密码失败：userID=%s, err=%v", user.ID, syncErr)
			}
		}
		return nil
	}

	return errors.New("用户名或密码错误")
}

func (s *AuthService) syncLocalLoginHash(user *models.User, password string) {
	if user.CheckPassword(password) {
		return
	}
	if err := user.SetPassword(password); err != nil {
		log.Printf("⚠️  登录时更新本地密码哈希失败（不影响登录）：userID=%s, err=%v", user.ID, err)
		return
	}
	if err := s.saveUser(user); err != nil {
		log.Printf("⚠️  登录时保存本地密码哈希失败（不影响登录）：userID=%s, err=%v", user.ID, err)
	}
}
