import test from 'node:test'
import assert from 'node:assert/strict'

import type { AdminConfigItem } from '../../types/api.js'
import {
  buildConfigUpdatePayload,
  buildDraftValues,
  canClearConfigOverride,
  getClearConfigLabel,
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

test('buildDraftValues keeps sensitive fields blank', () => {
  const draftValues = buildDraftValues([
    createItem({ key: 'SECRET_KEY', sensitive: true, type: 'secret', hasValue: true, value: undefined }),
  ])

  assert.equal(draftValues.SECRET_KEY, '')
})

test('isConfigItemDirty ignores json list order changes', () => {
  const item = createItem({
    type: 'json_list',
    value: JSON.stringify(['wechat_pay', 'card']),
  })

  assert.equal(isConfigItemDirty(item, ['card', 'wechat_pay']), false)
  assert.equal(isConfigItemDirty(item, ['card']), true)
})

test('buildConfigUpdatePayload skips blank sensitive overwrite', () => {
  const item = createItem({
    sensitive: true,
    type: 'secret',
    hasValue: true,
    value: undefined,
  })

  assert.equal(buildConfigUpdatePayload(item, ''), null)
  assert.deepEqual(buildConfigUpdatePayload(item, 'next-secret'), { value: 'next-secret' })
})

test('buildConfigUpdatePayload serializes structured values', () => {
  assert.deepEqual(buildConfigUpdatePayload(createItem({ type: 'boolean', value: 'false' }), true), {
    value: 'true',
  })
  assert.deepEqual(buildConfigUpdatePayload(createItem({ type: 'integer', value: '1' }), 7), {
    value: '7',
  })
  assert.deepEqual(
    buildConfigUpdatePayload(createItem({ type: 'json_list', value: '[]' }), ['card', 'alipay']),
    { value: JSON.stringify(['card', 'alipay']) }
  )
})

test('clear override action only appears for editable database-backed items', () => {
  assert.equal(canClearConfigOverride(createItem({ source: 'database' })), true)
  assert.equal(canClearConfigOverride(createItem({ source: 'env' })), false)
  assert.equal(canClearConfigOverride(createItem({ source: 'database', editable: false })), false)
  assert.equal(
    getClearConfigLabel(createItem({ source: 'database', sensitive: true, type: 'secret' })),
    '清空数据库覆盖值'
  )
})

test('hasExplicitEmptyDatabaseValue only matches explicit empty database overrides', () => {
  assert.equal(
    hasExplicitEmptyDatabaseValue(
      createItem({ source: 'database', allowEmpty: true, emptyValueMode: 'disable', hasValue: false, value: '' })
    ),
    true
  )
  assert.equal(
    hasExplicitEmptyDatabaseValue(
      createItem({ source: 'default', allowEmpty: true, emptyValueMode: 'disable', hasValue: false, value: '' })
    ),
    false
  )
})
