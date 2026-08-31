import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DevicesView from './DevicesView.vue'
import {
  getDeviceActions,
  getDeviceBlacklist,
  getDevices,
  getDeviceStats,
} from '@/api/admin'

vi.mock('@/api/admin', () => ({
  addDeviceBlacklist: vi.fn(),
  getDeviceActions: vi.fn(),
  getDeviceBlacklist: vi.fn(),
  getDevices: vi.fn(),
  getDeviceStats: vi.fn(),
  logoutBlacklistedDevices: vi.fn(),
  logoutDevice: vi.fn(),
  removeDeviceBlacklist: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
  },
  ElMessageBox: {
    confirm: vi.fn(),
  },
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const emptyStub = defineComponent({
  setup() {
    return () => null
  },
})

const EmberPageHeaderCardStub = defineComponent({
  props: {
    title: { type: String, default: '' },
  },
  setup(props, { slots }) {
    return () =>
      h('section', [
        h('h1', props.title),
        slots.titleSuffix?.(),
        slots.actions?.(),
        slots.default?.(),
      ])
  },
})

const EmberFilterPanelStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', [slots.default?.(), slots.actions?.()])
  },
})

const EmberTableCardStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
  },
  setup(props, { slots }) {
    return () =>
      h('section', [
        h(
          'div',
          props.data.map((row: any) => h('div', { class: 'device-row' }, row.deviceName))
        ),
        slots.pagination?.(),
        slots.default?.(),
      ])
  },
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function mockDeviceResponses() {
  vi.mocked(getDevices).mockResolvedValue({
    data: [
      {
        deviceId: 'device-1',
        deviceName: 'Apple TV',
        clientName: 'Infuse',
        userId: 'user-1',
        userName: 'demo',
        isActive: true,
        isBlacklisted: false,
        lastActivityDate: '2026-01-01T00:00:00Z',
      },
    ],
    total: 1,
    page: 1,
    pageSize: 20,
    totalPages: 1,
  } as never)
  vi.mocked(getDeviceStats).mockResolvedValue({
    data: {
      activeSessionCount: 1,
      blacklistedClientCount: 1,
      clientDistribution: [{ clientName: 'Infuse', count: 1 }],
      topDevices: [{ deviceName: 'Apple TV', count: 1 }],
    },
  } as never)
  vi.mocked(getDeviceBlacklist).mockResolvedValue({
    data: [
      {
        clientName: 'Bad Client',
        reason: 'blocked',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
  } as never)
  vi.mocked(getDeviceActions).mockResolvedValue({
    data: [
      {
        id: 'action-1',
        action: 'blacklist',
        clientName: 'Bad Client',
        deviceId: 'device-2',
        userId: 'user-2',
        note: 'blocked',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
  } as never)
}

function mountView() {
  return mount(DevicesView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberMetricCard: passthroughStub,
        EmberTableCard: EmberTableCardStub,
        EmberSearchInput: passthroughStub,
        EmberSelectField: passthroughStub,
        EmberFilterPanel: EmberFilterPanelStub,
        EmberPageHeaderCard: EmberPageHeaderCardStub,
        'el-icon': passthroughStub,
        'el-option': passthroughStub,
        'el-input': passthroughStub,
        'el-table': passthroughStub,
        'el-table-column': emptyStub,
        'el-tag': passthroughStub,
        'el-pagination': passthroughStub,
      },
    },
  })
}

describe('DevicesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockDeviceResponses()
  })

  it('keeps search scoped to device list and exposes a full refresh action for all panels', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getDevices).toHaveBeenCalledTimes(1)
    expect(getDeviceStats).toHaveBeenCalledTimes(1)
    expect(getDeviceBlacklist).toHaveBeenCalledTimes(1)
    expect(getDeviceActions).toHaveBeenCalledTimes(1)

    const vm = wrapper.vm as unknown as {
      query: { userId: string; page: number }
      handleDeviceSearch: () => void
      handleFullRefresh: () => Promise<void>
    }
    vm.query.userId = 'user-2'
    vm.handleDeviceSearch()
    await flushPromises()

    expect(getDevices).toHaveBeenCalledTimes(2)
    expect(getDeviceStats).toHaveBeenCalledTimes(1)
    expect(getDeviceBlacklist).toHaveBeenCalledTimes(1)
    expect(getDeviceActions).toHaveBeenCalledTimes(1)

    const refreshButton = wrapper.find('button[aria-label="刷新设备管理全部数据"]')
    expect(refreshButton.exists()).toBe(true)
    expect(refreshButton.text()).toContain('刷新')
    expect(refreshButton.attributes('disabled')).toBeUndefined()

    await refreshButton.trigger('click')
    await flushPromises()

    expect(vm.query.userId).toBe('user-2')
    expect(vm.query.page).toBe(1)
    expect(getDevices).toHaveBeenCalledTimes(3)
    expect(getDeviceStats).toHaveBeenCalledTimes(2)
    expect(getDeviceBlacklist).toHaveBeenCalledTimes(2)
    expect(getDeviceActions).toHaveBeenCalledTimes(2)
    expect(getDevices).toHaveBeenLastCalledWith(
      expect.objectContaining({
        userId: 'user-2',
        page: 1,
        pageSize: 20,
      })
    )
  })

  it('disables the full refresh button while all refresh requests are pending', async () => {
    const wrapper = mountView()
    await flushPromises()

    const devices = deferred<Awaited<ReturnType<typeof getDevices>>>()
    const stats = deferred<Awaited<ReturnType<typeof getDeviceStats>>>()
    const blacklists = deferred<Awaited<ReturnType<typeof getDeviceBlacklist>>>()
    const actions = deferred<Awaited<ReturnType<typeof getDeviceActions>>>()
    vi.mocked(getDevices).mockReturnValueOnce(devices.promise)
    vi.mocked(getDeviceStats).mockReturnValueOnce(stats.promise)
    vi.mocked(getDeviceBlacklist).mockReturnValueOnce(blacklists.promise)
    vi.mocked(getDeviceActions).mockReturnValueOnce(actions.promise)

    const refreshButton = wrapper.find('button[aria-label="刷新设备管理全部数据"]')
    await refreshButton.trigger('click')

    expect(refreshButton.attributes('disabled')).toBeDefined()
    expect(refreshButton.text()).toContain('刷新中')

    devices.resolve({
      data: [],
      total: 0,
      page: 1,
      pageSize: 20,
      totalPages: 0,
    } as never)
    stats.resolve({
      data: {
        activeSessionCount: 0,
        blacklistedClientCount: 0,
        clientDistribution: [],
        topDevices: [],
      },
    } as never)
    blacklists.resolve({ data: [] } as never)
    actions.resolve({ data: [] } as never)
    await flushPromises()

    expect(refreshButton.attributes('disabled')).toBeUndefined()
    expect(refreshButton.text()).toContain('刷新')
  })
})
