package auth

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// LoginRequest 统一登录请求
type LoginRequest struct {
	Username       string          `json:"username" binding:"required"`
	Password       string          `json:"password" binding:"required"`
	TurnstileToken string          `json:"turnstileToken"`
	ClientIP       string          `json:"-" swaggerignore:"true"`
	RequestContext context.Context `json:"-" swaggerignore:"true"`
}

// LoginResponse 统一登录响应
type LoginResponse struct {
	Token                 string       `json:"token"`
	User                  *models.User `json:"user"`
	IsExpired             bool         `json:"isExpired"`
	PasswordResetRequired bool         `json:"passwordResetRequired,omitempty"`
}

func (s *AuthService) Login(req *LoginRequest) (*LoginResponse, error) {
	if err := s.verifyTurnstileForLogin(req); err != nil {
		return nil, err
	}

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
		Token:                 token,
		User:                  user,
		IsExpired:             user.IsExpired(),
		PasswordResetRequired: user.PasswordResetRequired,
	}, nil
}

func (s *AuthService) findLoginUser(username string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("lower(username) = ?", strings.ToLower(username)).
		Order("\"createdAt\" ASC").
		First(&user)
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
	if err == nil {
		if embyUser.ID != user.EmbyID {
			log.Printf("[Login] EmbyID 错配 username=%s localEmbyID=%s remoteEmbyID=%s", user.Username, user.EmbyID, embyUser.ID)
			return errors.New("用户名或密码错误")
		}
		s.syncLocalLoginHash(user, password)
		return nil
	}

	if user.Password != "" && user.CheckPassword(password) {
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
