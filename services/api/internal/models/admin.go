package models

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Admin 管理员模型
// 对应 Prisma schema 的 Admin 模型
type Admin struct {
	ID        string    `json:"id" gorm:"type:varchar(25);primaryKey"` // cuid (25字符)
	Username  string    `json:"username" gorm:"uniqueIndex;size:50;not null"`
	Password  string    `json:"-" gorm:"not null"` // 永不序列化密码
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (Admin) TableName() string {
	return "admins"
}

// SetPassword 设置加密密码
func (a *Admin) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	a.Password = string(hash)
	return nil
}

// CheckPassword 验证密码
func (a *Admin) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(a.Password), []byte(password))
	return err == nil
}

// BeforeCreate 钩子：自动生成 cuid
func (a *Admin) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = generateCUID()
	}
	return nil
}
