import { describe, expect, it } from 'vitest'
import { formatCnyReferencePrice } from './reference-price'

describe('人民币参考价', () => {
  it('港币分转换为人民币元，固定汇率 0.85 并保留两位小数', () => {
    expect(formatCnyReferencePrice(5000, 'hkd')).toBe('约 ¥42.50')
    expect(formatCnyReferencePrice(999, 'HKD')).toBe('约 ¥8.49')
    expect(formatCnyReferencePrice(0, 'hkd')).toBe('约 ¥0.00')
  })

  it('其他币种和无效价格不显示参考金额', () => {
    for (const currency of ['cny', 'usd', 'eur', '']) {
      expect(formatCnyReferencePrice(5000, currency)).toBeNull()
    }
    for (const price of [-1, NaN, Infinity, 1.5]) {
      expect(formatCnyReferencePrice(price, 'hkd')).toBeNull()
    }
  })
})
