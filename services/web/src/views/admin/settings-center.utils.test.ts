import { describe, expect, it } from 'vitest'

import type { AdminConfigItem } from '../../types/api.js'
import {
  buildConfigUpdatePayload,
  buildDraftValues,
  hasExplicitEmptyDatabaseValue,
  isConfigItemDirty,
} from './settings-center.utils.js'

function createItem(overrides: Partial<AdminConfigItem> = {}): AdminConfigItem {
  return {
    key: 'TEST_KEY',
    group: 'business',
    groupLabel: '基础业务',
    label: '测试配置',
    description: '测试描述',
    type: 'string',
    multiline: false,
    editable: true,
    sensitive: false,
    restartRequired: false,
    allowEmpty: false,
    emptyValueMode: 'not_allowed',
    missingValueLevel: 'none',
    source: 'default',
    hasValue: true,
    value: 'value',
    ...overrides,
  }
}

describe('settings-center utils', () => {
  it('buildDraftValues keeps sensitive fields blank', () => {
    const draftValues = buildDraftValues([
      createItem({ key: 'SECRET_KEY', sensitive: true, type: 'secret', hasValue: true, value: undefined }),
    ])

    expect(draftValues.SECRET_KEY).toBe('')
  })

  it('isConfigItemDirty ignores json list order changes', () => {
    const item = createItem({
      type: 'json_list',
      value: JSON.stringify(['wechat_pay', 'card']),
    })

    expect(isConfigItemDirty(item, ['card', 'wechat_pay'])).toBe(false)
    expect(isConfigItemDirty(item, ['card'])).toBe(true)
  })

  it('buildConfigUpdatePayload skips blank sensitive overwrite', () => {
    const item = createItem({
      sensitive: true,
      type: 'secret',
      hasValue: true,
      value: undefined,
    })

    expect(buildConfigUpdatePayload(item, '')).toBeNull()
    expect(buildConfigUpdatePayload(item, 'next-secret')).toEqual({ value: 'next-secret' })
  })

  it('buildConfigUpdatePayload serializes structured values', () => {
    expect(buildConfigUpdatePayload(createItem({ type: 'boolean', value: 'false' }), true)).toEqual({
      value: 'true',
    })
    expect(buildConfigUpdatePayload(createItem({ type: 'integer', value: '1' }), 7)).toEqual({
      value: '7',
    })
    expect(
      buildConfigUpdatePayload(createItem({ type: 'json_list', value: '[]' }), ['card', 'alipay'])
    ).toEqual({ value: JSON.stringify(['card', 'alipay']) })
  })

  it('hasExplicitEmptyDatabaseValue only matches explicit empty database overrides', () => {
    expect(
      hasExplicitEmptyDatabaseValue(
        createItem({ source: 'database', allowEmpty: true, emptyValueMode: 'disable', hasValue: false, value: '' })
      )
    ).toBe(true)
    expect(
      hasExplicitEmptyDatabaseValue(
        createItem({ source: 'default', allowEmpty: true, emptyValueMode: 'disable', hasValue: false, value: '' })
      )
    ).toBe(false)
  })
})
