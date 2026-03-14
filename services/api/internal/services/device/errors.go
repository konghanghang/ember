package device

import "errors"

var (
	ErrClientBlacklistNotFound  = errors.New("客户端黑名单不存在")
	ErrDeviceClientNameRequired = errors.New("客户端名称不能为空")
	ErrDeviceIDRequired         = errors.New("设备 ID 不能为空")
)
