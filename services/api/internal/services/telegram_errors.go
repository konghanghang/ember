package services

import "errors"

var (
	ErrTelegramAlreadyBound     = errors.New("该 Telegram 账号已绑定其他用户")
	ErrTelegramBindCodeInvalid  = errors.New("绑定验证码无效或已过期")
	ErrTelegramNotBound         = errors.New("尚未绑定 Telegram 账号")
	ErrUserAlreadyBoundTelegram = errors.New("该账号已绑定 Telegram")
)
