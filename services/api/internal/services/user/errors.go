package user

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrRequestInvalid              = errors.New("请求参数错误")
	ErrUserNotFound                = errors.New("用户不存在")
	ErrUsernameLengthInvalid       = errors.New("用户名长度必须为 3-50 位")
	ErrUsernameCharsetInvalid      = errors.New("用户名只能包含字母和数字")
	ErrEmailRequired               = errors.New("邮箱不能为空")
	ErrEmailInvalid                = errors.New("邮箱格式错误")
	ErrPasswordTooShort            = errors.New("密码长度不能小于 6 位")
	ErrNeverExpireConflict         = errors.New("neverExpire=true 时不能再传 expiresAt")
	ErrExpiresAtRequired           = errors.New("neverExpire=false 时必须传 expiresAt")
	ErrExpiresAtFormatInvalid      = errors.New("expiresAt 必须是 RFC3339 格式")
	ErrUsernameAlreadyExists       = errors.New("用户名已存在")
	ErrEmailAlreadyExists          = errors.New("邮箱已存在")
	ErrUpdateFieldsRequired        = errors.New("至少提供一个可更新字段")
	ErrClearExpiresAtConflict      = errors.New("clearExpiresAt 和 expiresAt 不能同时设置")
	ErrOldPasswordInvalid          = errors.New("旧密码错误")
	ErrUserEmbyIDRequired          = errors.New("密码更新失败：用户缺少 Emby ID")
	ErrUserUpdateFailed            = errors.New("更新失败")
	ErrUserCreateFailed            = errors.New("创建用户失败")
	ErrUserResetPasswordFailed     = errors.New("重置密码失败")
	ErrUserPasswordUpdateFailed    = errors.New("密码更新失败")
	ErrUserPasswordLocalUpdateFail = errors.New("本地密码更新失败")
	ErrUserPasswordLocalSaveFail   = errors.New("本地密码保存失败")
)

func isUserUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}

	column = strings.ToLower(strings.TrimSpace(column))
	if column == "" {
		return true
	}

	return strings.Contains(strings.ToLower(pgErr.ConstraintName), column) ||
		strings.Contains(strings.ToLower(pgErr.Detail), column)
}
