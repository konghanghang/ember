package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 统一用户模型（admin + user）
// role 字段区分角色：admin 使用本地密码，user 通过 Emby 认证
type User struct {
	ID           string     `json:"id" gorm:"column:id;type:varchar(25);primaryKey"`
	Username     string     `json:"username" gorm:"column:username;uniqueIndex;size:50;not null"`
	Role         string     `json:"role" gorm:"column:role;size:10;not null;default:user"`
	Password     string     `json:"-" gorm:"column:password"` // bcrypt hash，所有用户通用
	Email        string     `json:"email,omitempty" gorm:"column:email;size:255;uniqueIndex"`
	EmbyID       string     `json:"embyId,omitempty" gorm:"column:embyId;size:50;index"`
	EmbyDisabled bool       `json:"embyDisabled" gorm:"column:embyDisabled;default:false;not null"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty" gorm:"column:expiresAt"`
	IsActive     bool       `json:"isActive" gorm:"column:isActive;default:true;not null"`
	CreatedAt    time.Time  `json:"createdAt" gorm:"column:createdAt;autoCreateTime"`
	UpdatedAt    time.Time  `json:"updatedAt" gorm:"column:updatedAt;autoUpdateTime"`
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
	return u.ExpiresAt.Before(time.Now())
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
