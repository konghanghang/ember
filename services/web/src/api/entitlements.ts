import { request } from './request'
import type { PlanBenefit, UserEntitlement } from '@/types/api'

export interface EntitlementAdjustment {
  operationId: string
  planGroup: string
  action: 'set' | 'extend' | 'revoke'
  validityType?: 'duration' | 'permanent'
  expiresAt?: string | null
  days?: number
}

/** 读取用户实际持有的权益，不在浏览时触发到期回退。 */
export function getEntitlements(userId?: string): Promise<{ data: UserEntitlement[]; businessTimezone: string }> {
  return request({ url: userId ? `/admin/users/${encodeURIComponent(userId)}/entitlements` : '/user/entitlements', method: 'get' })
}

/** 对指定分组执行幂等的管理员权益调整。 */
export function adjustEntitlement(userId: string, data: EntitlementAdjustment): Promise<void> {
  return request({ url: `/admin/users/${encodeURIComponent(userId)}/entitlements`, method: 'post', data })
}

/** 显式保存所有分组的权益等级，服务端校验资源库逐级包含。 */
export function setEntitlementRanks(ranks: Record<string, number>): Promise<void> {
  return request({ url: '/admin/plan-groups/ranks', method: 'put', data: { ranks } })
}

/** 记录线下已退款或发放补偿；退款选项不会调用支付平台。 */
export function resolvePayment(id: string, resolution: 'external_refund' | 'compensation', note: string, benefits?: PlanBenefit[]): Promise<void> {
  return request({ url: `/admin/payments/${encodeURIComponent(id)}/resolve`, method: 'post', data: { resolution, note, benefits } })
}
