package config

import "errors"

var (
	ErrConfigNotFound                    = errors.New("配置项不存在")
	ErrConfigNotEditable                 = errors.New("该配置项不允许在线编辑")
	ErrConfigValueRequired               = errors.New("缺少配置值")
	ErrConfigEncryptionKeyMissing        = errors.New("未配置 CONFIG_ENCRYPTION_KEY，无法保存敏感配置")
	ErrConfigGroupUnsupported            = errors.New("不支持的配置分组测试")
	ErrConfigSensitiveReadForbidden      = errors.New("敏感配置项不支持明文读取")
	ErrPaymentMethodSettingInvalid       = errors.New("支付方式配置无效")
	ErrConfigValidation                  = errors.New("配置校验失败")
	ErrRegistrationEmailDomainNotAllowed = errors.New("当前注册仅允许特定邮箱域名，请使用允许的邮箱注册")
	ErrRegistrationEmailInvalid          = errors.New("邮箱格式无效")
)
