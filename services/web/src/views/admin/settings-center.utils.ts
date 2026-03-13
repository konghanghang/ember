import type { AdminConfigItem, UpdateAdminConfigRequest } from '../../types/api.js'

export type ConfigDraftValue = boolean | number | string | string[]

export function parseDraftValue(item: AdminConfigItem): ConfigDraftValue {
  if (item.sensitive) {
    return ''
  }

  switch (item.type) {
    case 'boolean':
      return item.value === 'true'
    case 'integer':
      return Number(item.value ?? 0)
    case 'json_list':
      if (!item.value) {
        return []
      }
      try {
        const parsed = JSON.parse(item.value)
        return Array.isArray(parsed) ? parsed : []
      } catch {
        return []
      }
    default:
      return item.value ?? ''
  }
}

export function buildDraftValues(items: AdminConfigItem[]): Record<string, ConfigDraftValue> {
  const next: Record<string, ConfigDraftValue> = {}

  for (const item of items) {
    next[item.key] = parseDraftValue(item)
  }

  return next
}

export function normalizeComparableValue(item: AdminConfigItem, value: unknown): string | number | boolean {
  switch (item.type) {
    case 'boolean':
      return value === true
    case 'integer':
      return Number(value ?? 0)
    case 'json_list':
      return JSON.stringify([...(Array.isArray(value) ? value : [])].sort())
    default:
      return String(value ?? '')
  }
}

export function currentComparableValue(item: AdminConfigItem): string | number | boolean {
  return normalizeComparableValue(item, parseDraftValue(item))
}

export function isConfigItemDirty(item: AdminConfigItem, draftValue: unknown): boolean {
  if (item.sensitive) {
    return String(draftValue ?? '').trim() !== ''
  }
  return normalizeComparableValue(item, draftValue) !== currentComparableValue(item)
}

export function buildConfigUpdatePayload(
  item: AdminConfigItem,
  draftValue: unknown
): UpdateAdminConfigRequest | null {
  if (item.sensitive) {
    const raw = String(draftValue ?? '')
    if (raw.trim() === '') {
      return null
    }
    return { value: raw }
  }

  switch (item.type) {
    case 'boolean':
      return { value: String(Boolean(draftValue)) }
    case 'integer':
      return { value: String(Number(draftValue ?? 0)) }
    case 'json_list': {
      const values = Array.isArray(draftValue) ? draftValue : []
      return { value: values.length === 0 ? '' : JSON.stringify(values) }
    }
    default:
      return { value: String(draftValue ?? '') }
  }
}

export function canClearConfigOverride(item: AdminConfigItem): boolean {
  return item.editable && item.source === 'database'
}

export function getClearConfigLabel(item: AdminConfigItem): string {
  return item.sensitive ? '清空数据库覆盖值' : '移除数据库覆盖值'
}

export function getClearConfigDescription(item: AdminConfigItem): string {
  if (item.sensitive) {
    return '删除当前数据库中的敏感覆盖值，并回退到环境变量、默认值或未设置状态。'
  }
  return '删除当前数据库覆盖值，并回退到环境变量、默认值或未设置状态。'
}
