package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 统一用户模型（admin + user）
// role 字段区分角色：admin 使用本地密码，user 通过 Emby 认证
// username/email 的唯一性由 SQL 层 lower(...) 函数唯一索引维护；线上不依赖 GORM AutoMigrate。
type User struct {
	ID                    string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Username              string     `json:"username" gorm:"column:username;size:50;not null"`
	Role                  string     `json:"role" gorm:"column:role;size:10;not null;default:user"`
	Password              string     `json:"-" gorm:"column:password"` // bcrypt hash，所有用户通用
	Email                 string     `json:"email,omitempty" gorm:"column:email;size:255"`
	EmbyID                string     `json:"embyId,omitempty" gorm:"column:emby_id;size:50;index"`
	EmbyDisabled          bool       `json:"embyDisabled" gorm:"column:emby_disabled;default:false;not null"`
	TelegramID            *int64     `json:"telegramId,omitempty" gorm:"column:telegram_id"` // partial unique 由 uq_users_telegram_id 维护
	PlanGroup             *string    `json:"planGroup,omitempty" gorm:"column:plan_group;size:50;index"`
	ExpiresAt             *time.Time `json:"expiresAt,omitempty" gorm:"column:expires_at"`
	IsActive              bool       `json:"isActive" gorm:"column:is_active;default:true;not null"`
	PasswordResetRequired bool       `json:"passwordResetRequired,omitempty" gorm:"column:password_reset_required;default:false"`
	CreatedAt             time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt             time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (User) TableName() string {
	return "users"
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = generateCUID()
	}
	return nil
}

// IsExpired 检查账号是否过期
func (u *User) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return u.ExpiresAt.Before(time.Now().UTC())
}

// IsAdmin 是否为管理员
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// SetPassword 设置加密密码（admin 专用）
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

// CheckPassword 验证密码（admin 专用）
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}
