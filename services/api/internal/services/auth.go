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

	// 4. 验证 Emby 密码
	embyService := NewEmbyService()
	embyUser, err := embyService.AuthenticateUser(user.Username, req.Password)
	if err != nil {
		return nil, errors.New("密码错误")
	}

	// 5. 验证 Emby ID 是否匹配
	if embyUser.ID != user.EmbyID {
		return nil, errors.New("用户信息不匹配")
	}

	// 6. 生成 JWT Token
	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &UserLoginResponse{
		Token: token,
		User:  &user,
	}, nil
}

// RegisterUserRequest 用户注册请求
type RegisterUserRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=50"`
	Password   string `json:"password" binding:"required,min=6"`
	Email      string `json:"email" binding:"required,email"`
	InviteCode string `json:"inviteCode" binding:"required"`
}

// RegisterUserResponse 用户注册响应
type RegisterUserResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// RegisterUser 用户注册
func (s *AuthService) RegisterUser(req *RegisterUserRequest) (*RegisterUserResponse, error) {
	// 1. 验证邀请码
	inviteService := &InviteService{}
	invite, err := inviteService.ValidateInvite(req.InviteCode)
	if err != nil {
		return nil, err
	}

	// 2. 检查用户名是否已存在
	var existingUser models.User
	result := db.DB.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		return nil, errors.New("用户名已存在")
	}

	// 3. 创建 Emby 用户
	embyService := NewEmbyService()
	embyUser, err := embyService.CreateEmbyUser(req.Username, req.Password)
	if err != nil {
		return nil, errors.New("创建 Emby 用户失败：" + err.Error())
	}

	// 4. 计算到期时间
	expiresAt := common.CalculateExpiryDate(invite.DefaultDays)

	// 5. 创建数据库用户记录
	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		EmbyID:     embyUser.ID,
		InviteCode: req.InviteCode,
		ExpiresAt:  &expiresAt,
		IsActive:   true,
	}

	if err := db.DB.Create(&user).Error; err != nil {
		// 如果数据库创建失败，理论上应该删除 Emby 用户
		// 但这里简化处理，后续可以添加清理逻辑
		return nil, errors.New("创建用户失败")
	}

	// 6. 使用邀请码（增加使用次数）
	if err := inviteService.UseInvite(req.InviteCode); err != nil {
		// 记录错误但不影响注册流程
		// log.Printf("警告：邀请码使用次数更新失败：%v", err)
	}

	// 7. 生成 JWT Token
	token, err := common.GenerateToken(user.ID, user.Username, "user")
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}

	return &RegisterUserResponse{
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
