package email

import "errors"

var (
	ErrEmailNotConfigured     = errors.New("邮件服务未配置")
	ErrEmailAlreadyRegistered = errors.New("邮箱已被注册")
	ErrEmailAlreadyBound      = errors.New("邮箱已被其他用户绑定")
	ErrEmailUnchanged         = errors.New("新邮箱与当前邮箱相同")
	ErrEmailNotRegistered     = errors.New("该邮箱未注册")
	ErrEmailCodeRateLimit     = errors.New("该邮箱今日发送次数已达上限")
	ErrEmailCodeIPRateLimit   = errors.New("请求过于频繁，请稍后再试")
	ErrEmailCodeInvalid       = errors.New("邮箱验证码无效或已过期")
	ErrEmailSendFailed        = errors.New("验证码发送失败，请稍后重试")
)
