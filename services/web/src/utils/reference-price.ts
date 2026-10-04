// 管理员确认的固定展示汇率；未来引入汇率配置时替换，不参与支付金额计算。
const HKD_TO_CNY_REFERENCE_RATE = 0.85

/** 将港币最小货币单位换算为人民币参考文案；其他币种或无效价格不展示。 */
export function formatCnyReferencePrice(price: number, currency: string): string | null {
  if (currency.toLowerCase() !== 'hkd' || !Number.isSafeInteger(price) || price < 0) return null
  const amount = (price / 100 * HKD_TO_CNY_REFERENCE_RATE).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })
  return `约 ¥${amount}`
}
