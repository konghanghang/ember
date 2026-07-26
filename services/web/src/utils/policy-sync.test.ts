import { describe, expect, it } from 'vitest'

import {
  resolveAccountPolicySyncPresentation,
  resolveGroupPolicySyncPresentation,
  resolveUserPolicySyncPresentation
} from './policy-sync'
import type { EmbyPolicySyncStatus } from '@/types/api'

const ALL_STATUSES: EmbyPolicySyncStatus[] = [
  'pending',
  'processing',
  'synced',
  'partial_failed',
  'failed',
  'out_of_sync'
]

describe('resolveUserPolicySyncPresentation（单用户视角）', () => {
  it('pending/processing 合并为同步中', () => {
    expect(resolveUserPolicySyncPresentation('pending')).toEqual({ label: '同步中', tagType: 'warning' })
    expect(resolveUserPolicySyncPresentation('processing')).toEqual({ label: '同步中', tagType: 'warning' })
  })

  it('失败类状态映射为 danger', () => {
    expect(resolveUserPolicySyncPresentation('failed')).toEqual({ label: '同步失败', tagType: 'danger' })
    expect(resolveUserPolicySyncPresentation('partial_failed')).toEqual({ label: '部分失败', tagType: 'danger' })
  })

  it('out_of_sync 显示待同步，synced 显示已同步', () => {
    expect(resolveUserPolicySyncPresentation('out_of_sync')).toEqual({ label: '待同步', tagType: 'warning' })
    expect(resolveUserPolicySyncPresentation('synced')).toEqual({ label: '已同步', tagType: 'success' })
  })

  it('状态缺失时按已同步兜底（历史 else 分支行为）', () => {
    expect(resolveUserPolicySyncPresentation(undefined)).toEqual({ label: '已同步', tagType: 'success' })
    expect(resolveUserPolicySyncPresentation(null)).toEqual({ label: '已同步', tagType: 'success' })
  })
})

describe('resolveAccountPolicySyncPresentation（终端用户视角）', () => {
  it('partial_failed 折叠为已同步，不暴露内部部分失败', () => {
    expect(resolveAccountPolicySyncPresentation('partial_failed')).toEqual({ label: '已同步', tagType: 'success' })
  })

  it('其余状态与单用户视角一致', () => {
    for (const status of ALL_STATUSES) {
      if (status === 'partial_failed') continue
      expect(resolveAccountPolicySyncPresentation(status)).toEqual(resolveUserPolicySyncPresentation(status))
    }
    expect(resolveAccountPolicySyncPresentation(undefined)).toEqual(resolveUserPolicySyncPresentation(undefined))
  })
})

describe('resolveGroupPolicySyncPresentation（分组/批次视角）', () => {
  it('pending 表示批次未开始，显示待同步', () => {
    expect(resolveGroupPolicySyncPresentation('pending')).toEqual({ label: '待同步', tagType: 'warning' })
    expect(resolveGroupPolicySyncPresentation('out_of_sync')).toEqual({ label: '待同步', tagType: 'warning' })
  })

  it('processing/synced/失败类状态逐一映射', () => {
    expect(resolveGroupPolicySyncPresentation('processing')).toEqual({ label: '同步中', tagType: 'warning' })
    expect(resolveGroupPolicySyncPresentation('synced')).toEqual({ label: '已同步', tagType: 'success' })
    expect(resolveGroupPolicySyncPresentation('partial_failed')).toEqual({ label: '部分失败', tagType: 'danger' })
    expect(resolveGroupPolicySyncPresentation('failed')).toEqual({ label: '失败', tagType: 'danger' })
  })

  it('状态缺失时显示未知', () => {
    expect(resolveGroupPolicySyncPresentation(undefined)).toEqual({ label: '未知', tagType: 'info' })
    expect(resolveGroupPolicySyncPresentation(null)).toEqual({ label: '未知', tagType: 'info' })
  })

  it('覆盖全部枚举状态，无遗漏分支', () => {
    for (const status of ALL_STATUSES) {
      const presentation = resolveGroupPolicySyncPresentation(status)
      expect(presentation.label).toBeTruthy()
      expect(['success', 'warning', 'danger', 'info']).toContain(presentation.tagType)
    }
  })
})
