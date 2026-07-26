import type { EmbyPolicySyncStatus } from '@/types/api'

export type PolicySyncTagType = 'success' | 'warning' | 'danger' | 'info'

export interface PolicySyncPresentation {
  /** 标签文案 */
  label: string
  /** Element Plus el-tag 语义类型 */
  tagType: PolicySyncTagType
}

/**
 * 逐字对比三处历史实现后的结论（2026-07-26）：
 * - UsersView（管理员看单个用户）与 AccountCenterView（用户看自己）共享一套口径：
 *   pending/processing 合并显示「同步中」，failed 显示「同步失败」。
 * - AccountCenterView 额外把 partial_failed 折叠为「已同步」：终端用户不需要感知
 *   部分失败这种内部细节，历史模板里该状态落到 else 分支，此处保留原行为。
 * - PlanGroupsView（管理员看分组/批次）语义不同：pending 表示批次尚未开始，显示
 *   「待同步」；failed 用短文案「失败」；未知状态兜底「未知」。
 * 三套口径是上下文语义差异，统一数据结构、保留具名变体，禁止再互相抄改。
 */

const isSyncingStatus = (status?: EmbyPolicySyncStatus | null): boolean => {
  return status === 'pending' || status === 'processing'
}

/**
 * 单用户视角的同步状态展示（UsersView 用户行）。
 * pending/processing 合并为「同步中」；未返回状态时按「已同步」兜底，与历史 else 分支一致。
 */
export function resolveUserPolicySyncPresentation(status?: EmbyPolicySyncStatus | null): PolicySyncPresentation {
  if (isSyncingStatus(status)) return { label: '同步中', tagType: 'warning' }
  switch (status) {
    case 'failed':
      return { label: '同步失败', tagType: 'danger' }
    case 'partial_failed':
      return { label: '部分失败', tagType: 'danger' }
    case 'out_of_sync':
      return { label: '待同步', tagType: 'warning' }
    default:
      return { label: '已同步', tagType: 'success' }
  }
}

/**
 * 终端用户视角的同步状态展示（AccountCenterView 媒体库偏好）。
 * 与单用户视角一致，但 partial_failed 折叠为「已同步」，不向用户暴露内部部分失败细节。
 */
export function resolveAccountPolicySyncPresentation(status?: EmbyPolicySyncStatus | null): PolicySyncPresentation {
  if (status === 'partial_failed') return { label: '已同步', tagType: 'success' }
  return resolveUserPolicySyncPresentation(status)
}

/**
 * 分组/批次视角的同步状态展示（PlanGroupsView 分组表格）。
 * pending 表示批次尚未开始（「待同步」），failed 用短文案；未知状态兜底「未知」。
 */
export function resolveGroupPolicySyncPresentation(status?: EmbyPolicySyncStatus | null): PolicySyncPresentation {
  if (status === 'pending' || status === 'out_of_sync') return { label: '待同步', tagType: 'warning' }
  if (status === 'processing') return { label: '同步中', tagType: 'warning' }
  if (status === 'partial_failed') return { label: '部分失败', tagType: 'danger' }
  if (status === 'failed') return { label: '失败', tagType: 'danger' }
  if (status === 'synced') return { label: '已同步', tagType: 'success' }
  return { label: '未知', tagType: 'info' }
}
