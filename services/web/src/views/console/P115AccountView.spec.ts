import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import P115AccountView from './P115AccountView.vue'
import type { PersonalP115Account } from '@/types/api'
import {
  createPersonalP115Account,
  getPersonalP115Account,
  getPersonalP115Usage,
  replacePersonalP115Cookie,
  revokePersonalP115Account,
  setPersonalP115Enabled,
  updatePersonalP115Concurrency,
  updatePersonalP115Directory,
  validatePersonalP115Account,
} from '@/api/console'
import { ElMessageBox } from 'element-plus'

vi.mock('@/api/console', () => ({
  createPersonalP115Account: vi.fn(),
  getPersonalP115Account: vi.fn(),
  getPersonalP115Usage: vi.fn(),
  replacePersonalP115Cookie: vi.fn(),
  revokePersonalP115Account: vi.fn(),
  setPersonalP115Enabled: vi.fn(),
  updatePersonalP115Concurrency: vi.fn(),
  updatePersonalP115Directory: vi.fn(),
  validatePersonalP115Account: vi.fn(),
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
    return () => h('div', [slots.default?.(), slots.actions?.(), slots.titleSuffix?.()])
  },
})

const inputStub = defineComponent({
  props: {
    modelValue: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    type: { type: String, default: 'text' },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('input', {
      value: props.modelValue,
      type: props.type === 'textarea' ? 'text' : props.type,
      placeholder: props.placeholder,
      onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
    })
  },
})

const inputNumberStub = defineComponent({
  props: {
    modelValue: { type: Number, default: 1 },
    max: { type: Number, default: 100 },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('input', {
      value: String(props.modelValue),
      type: 'number',
      max: String(props.max),
      onInput: (event: Event) => emit('update:modelValue', Number((event.target as HTMLInputElement).value)),
    })
  },
})

function personalAccount(overrides: Partial<PersonalP115Account> = {}): PersonalP115Account {
  return {
    id: 'personal-1',
    appType: 'android',
    status: 'pending',
    enabled: false,
    usageAvailable: false,
    reservedStreams: null,
    activeStreams: null,
    occupiedStreams: null,
    createdAt: '2026-09-04T00:00:00Z',
    updatedAt: '2026-09-04T00:00:00Z',
    ...overrides,
  }
}

function mountView() {
  return mount(P115AccountView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberEmptyStateCard: passthroughStub,
        EmberPageHeaderCard: passthroughStub,
        'el-icon': passthroughStub,
        'el-input': inputStub,
        'el-input-number': inputNumberStub,
      },
    },
  })
}

function findButton(wrapper: ReturnType<typeof mountView>, label: string) {
  const button = wrapper.findAll('button').find(item => item.text().includes(label))
  expect(button, `未找到按钮: ${label}`).toBeTruthy()
  return button!
}

describe('P115AccountView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(createPersonalP115Account).mockResolvedValue(personalAccount())
    vi.mocked(getPersonalP115Usage).mockResolvedValue({
      p115PlaybackMode: 'personal',
      usageAvailable: true,
      userReservedStreams: 0,
      userActiveStreams: 0,
      userOccupiedStreams: 0,
      transferPending: 0,
      transferHourlyUsed: 0,
      transferHourlyLimit: 5,
      transferDailyUsed: 0,
      transferDailyLimit: 10,
    })
    vi.mocked(replacePersonalP115Cookie).mockResolvedValue(personalAccount())
    vi.mocked(validatePersonalP115Account).mockResolvedValue({ valid: true, account: personalAccount({ status: 'active' }) })
    vi.mocked(updatePersonalP115Directory).mockResolvedValue(personalAccount({ status: 'active', targetParentPath: '/Playback' }))
    vi.mocked(updatePersonalP115Concurrency).mockResolvedValue(personalAccount({ status: 'active', maxConcurrentStreams: 2 }))
    vi.mocked(setPersonalP115Enabled).mockResolvedValue(personalAccount({ status: 'active', enabled: true }))
    vi.mocked(revokePersonalP115Account).mockResolvedValue({ message: '115 账号已解绑' })
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm' as never)
  })

  it('未绑定时只要求 Cookie，创建成功后清空并重新读取完整摘要', async () => {
    vi.mocked(getPersonalP115Account)
      .mockRejectedValueOnce({ response: { status: 404 } })
      .mockResolvedValueOnce(personalAccount())

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('User-Agent')
    expect(wrapper.text()).not.toContain('客户端类型输入')
    const cookie = wrapper.get('input[placeholder="粘贴完整 Cookie"]')
    await cookie.setValue('UID=100_F1_1700000000')
    await findButton(wrapper, '绑定账号').trigger('click')
    await flushPromises()

    expect(createPersonalP115Account).toHaveBeenCalledWith('UID=100_F1_1700000000')
    expect(wrapper.find('input[placeholder="粘贴完整 Cookie"]').exists()).toBe(false)
    expect(wrapper.html()).not.toContain('UID=100_F1_1700000000')
    expect(getPersonalP115Account).toHaveBeenCalledTimes(2)
  })

  it('待验证账号只突出下一步验证，不提前开放配置和启用', async () => {
    vi.mocked(getPersonalP115Account).mockResolvedValue(personalAccount())
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('待验证')
    expect(findButton(wrapper, '验证 Cookie').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('input[placeholder="例如：/Ember/Playback"]').exists()).toBe(false)
    expect(findButton(wrapper, '启用账号').attributes('disabled')).toBeDefined()
  })

  it('验证请求失败后仍重读后端已经持久化的错误状态', async () => {
    vi.mocked(validatePersonalP115Account).mockRejectedValueOnce(new Error('provider unavailable'))
    vi.mocked(getPersonalP115Account)
      .mockResolvedValueOnce(personalAccount())
      .mockResolvedValueOnce(personalAccount({ status: 'error', lastErrorCode: 'provider_unavailable' }))
    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '验证 Cookie').trigger('click')
    await flushPromises()

    expect(getPersonalP115Account).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('验证异常')
  })

  it('验证后按当前套餐上限配置目录和并发', async () => {
    vi.mocked(getPersonalP115Account)
      .mockResolvedValueOnce(personalAccount({
        status: 'active',
        simultaneousStreamLimit: 3,
        p115PlaybackMode: 'personal',
        transferHourlyLimit: 5,
        transferDailyLimit: 10,
      }))
      .mockResolvedValue(personalAccount({
        status: 'active',
        targetParentPath: '/Playback',
        maxConcurrentStreams: 2,
        effectiveMaxConcurrentStreams: 2,
        simultaneousStreamLimit: 3,
      }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[placeholder="例如：/Ember/Playback"]').setValue('/Playback')
    await findButton(wrapper, '保存目录').trigger('click')
    await flushPromises()
    expect(updatePersonalP115Directory).toHaveBeenCalledWith('/Playback')

    const maxInput = wrapper.get('input[type="number"]')
    expect(maxInput.attributes('max')).toBe('3')
    await maxInput.setValue('2')
    await findButton(wrapper, '保存播放路数').trigger('click')
    await flushPromises()
    expect(updatePersonalP115Concurrency).toHaveBeenCalledWith(2)
  })

  it('套餐降低时分别展示配置值和有效值，不把较小值写回表单', async () => {
    vi.mocked(getPersonalP115Account).mockResolvedValue(personalAccount({
      status: 'active',
      targetParentPath: '/Playback',
      maxConcurrentStreams: 8,
      effectiveMaxConcurrentStreams: 3,
      simultaneousStreamLimit: 3,
    }))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('配置 8 路')
    expect(wrapper.text()).toContain('当前有效 3 路')
    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe('8')
  })

  it('用量不可用不会显示成零，未知客户端显示未识别', async () => {
    vi.mocked(getPersonalP115Account).mockResolvedValue(personalAccount({
      appType: 'unknown',
      status: 'active',
      usageAvailable: false,
    }))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('未识别')
    expect(wrapper.text()).toContain('用量不可用')
    expect(wrapper.text()).not.toContain('总占用 0')
  })

  it('展示本人播放归因和小时每日转存用量', async () => {
    vi.mocked(getPersonalP115Account).mockResolvedValue(personalAccount({
      status: 'active', usageAvailable: true,
      reservedStreams: 1, activeStreams: 2, occupiedStreams: 3,
    }))
    vi.mocked(getPersonalP115Usage).mockResolvedValue({
      p115PlaybackMode: 'system',
      usageAvailable: true,
      userReservedStreams: 1,
      userActiveStreams: 1,
      userOccupiedStreams: 2,
      transferPending: 1,
      transferHourlyUsed: 3,
      transferHourlyLimit: 5,
      transferDailyUsed: 4,
      transferDailyLimit: 10,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('本人播放中')
    expect(wrapper.text()).toContain('3 / 5')
    expect(wrapper.text()).toContain('4 / 10')
  })

  it('解绑需要确认，成功后回到未绑定状态', async () => {
    vi.mocked(getPersonalP115Account)
      .mockResolvedValueOnce(personalAccount({ status: 'active' }))
      .mockRejectedValueOnce({ response: { status: 404 } })
    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '解绑账号').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(revokePersonalP115Account).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('绑定账号')
  })
})
