package services

import (
	"errors"

	"github.com/konghang/ember/backend/internal/common"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// AuthService 认证服务
type AuthService struct{}

// AdminLoginRequest 管理员登录请求
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AdminLoginResponse 管理员登录响应
type AdminLoginResponse struct {
	Token string        `json:"token"`
	Admin *models.Admin `json:"admin"`
}

// AdminLogin 管理员登录
func (s *AuthService) AdminLogin(req *AdminLoginRequest) (*AdminLoginResponse, error) {
	// 1. 查询管理员
	var admin models.Admin
	result := db.DB.Where("username = ?", req.Username).First(&admin)
	if result.Error != nil {
		return nil, errors.New("用户名或密码错误")
	}

	// 2. 验证密码
	if !admin.CheckPassword(req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	// 3. 生成 JWT Token
	token, err := common.GenerateToken(admin.ID, admin.Username, "admin")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &AdminLoginResponse{
		Token: token,
		Admin: &admin,
	}, nil
}

// UserLoginRequest 用户登录请求
type UserLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// UserLoginResponse 用户登录响应
type UserLoginResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// UserLogin 用户登录（通过 Emby 验证）
// TODO: 实现 Emby API 验证
func (s *AuthService) UserLogin(req *UserLoginRequest) (*UserLoginResponse, error) {
	// 1. 查询用户
	var user models.User
	result := db.DB.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	// 2. 检查用户状态
	if !user.IsActive {
		return nil, errors.New("账号已被禁用")
	}

	// 3. 检查是否过期
	if user.IsExpired() {
		return nil, errors.New("账号已过期")
	}

	// 4. TODO: 验证 Emby 密码
	// embyService := &EmbyService{}
	// valid, err := embyService.ValidateUser(user.EmbyID, req.Password)
	// if err != nil || !valid {
	//     return nil, errors.New("密码错误")
	// }

	// 5. 生成 JWT Token
	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &UserLoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

// GetCurrentUser 获取当前用户信息（通过 Token）
func (s *AuthService) GetCurrentUser(userID string, role string) (interface{}, error) {
	if role == "admin" {
		var admin models.Admin
		result := db.DB.Where("id = ?", userID).First(&admin)
		if result.Error != nil {
			return nil, errors.New("用户不存在")
		}
		return &admin, nil
	} else {
		var user models.User
		result := db.DB.Where("id = ?", userID).First(&user)
		if result.Error != nil {
			return nil, errors.New("用户不存在")
		}
		return &user, nil
	}
}
