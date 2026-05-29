package redemption

import "errors"

var (
	ErrRedemptionCodeNotFound          = errors.New("兑换码不存在")
	ErrRedemptionCodeInvalid           = errors.New("兑换码已失效")
	ErrRedemptionCodeStatusInvalid     = errors.New("兑换码状态筛选值无效")
	ErrRegistrationPlanGroupRequired   = errors.New("注册套餐分组不能为空")
	ErrRegistrationPlanGroupNotFound   = errors.New("兑换码绑定的套餐分组不存在")
	ErrRedemptionDuplicate             = errors.New("你已经使用过此兑换码")
	ErrRedemptionCodeUsedOver          = errors.New("最大使用次数不能小于已使用次数")
	ErrRedemptionCodeBatchCountInvalid = errors.New("批量生成数量必须在 1 到 100 之间")
	ErrRedeemFailed                    = errors.New("兑换失败，请稍后重试")
	ErrEmbyUnbanFailed                 = errors.New("Emby 解封失败，请稍后重试")
)
