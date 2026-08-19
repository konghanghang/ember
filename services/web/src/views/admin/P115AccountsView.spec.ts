import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import P115AccountsView from './P115AccountsView.vue'
import type { P115Account } from '@/types/api'
import {
  createP115Account,
  getP115Accounts,
  replaceP115AccountCookie,
  setP115AccountEnabled,
  validateP115Account,
} from '@/api/admin'
import { ElMessage } from 'element-plus'

vi.mock('@/api/admin', () => ({
  createP115Account: vi.fn(),
  getP115Accounts: vi.fn(),
  replaceP115AccountCookie: vi.fn(),
  setP115AccountEnabled: vi.fn(),
  validateP115Account: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    success: vi.fn(),
    warning: vi.fn(),
  },
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', [slots.default?.(), slots.actions?.(), slots.titleSuffix?.()])
  },
})

const formDialogStub = defineComponent({
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () => props.modelValue
      ? h('section', { 'data-test': 'form-dialog', 'data-title': props.title }, [
          slots.default?.(),
          slots.footer?.(),
        ])
      : null
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

const selectStub = defineComponent({
  props: {
    modelValue: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, slots }) {
    return () => h('select', {
      value: props.modelValue,
      onChange: (event: Event) => emit('update:modelValue', (event.target as HTMLSelectElement).value),
    }, slots.default?.())
  },
})

const optionStub = defineComponent({
  props: {
    label: { type: String, default: '' },
    value: { type: String, default: '' },
  },
  setup(props) {
    return () => h('option', { value: props.value }, props.label)
  },
})

function account(overrides: Partial<P115Account> = {}): P115Account {
  return {
    id: 'p115_source',
    role: 'source',
    alias: '源账号',
    authMode: 'legacy_cookie',
    appType: 'web',
    userAgent: 'Ember Test',
    status: 'pending',
    enabled: false,
    createdAt: '2026-08-18T00:00:00Z',
    updatedAt: '2026-08-18T00:00:00Z',
    ...overrides,
  }
}

function mountView() {
  return mount(P115AccountsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        EmberPageHeaderCard: passthroughStub,
        EmberEmptyStateCard: passthroughStub,
        EmberFormDialog: formDialogStub,
        'el-icon': passthroughStub,
        'el-input': inputStub,
        'el-select': selectStub,
        'el-option': optionStub,
      },
    },
  })
}

function findButton(wrapper: ReturnType<typeof mountView>, label: string) {
  const button = wrapper.findAll('button').find(item => item.text().includes(label))
  expect(button, `未找到按钮: ${label}`).toBeTruthy()
  return button!
}

describe('P115AccountsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getP115Accounts).mockResolvedValue({ data: [account()] })
    vi.mocked(createP115Account).mockResolvedValue(account({ id: 'created' }))
    vi.mocked(validateP115Account).mockResolvedValue({
      valid: true,
      account: account({ status: 'active', providerUserId: '100' }),
    })
    vi.mocked(setP115AccountEnabled).mockResolvedValue(account({ status: 'active', enabled: true }))
    vi.mocked(replaceP115AccountCookie).mockResolvedValue(account())
  })

  it('展示安全账号摘要，pending 账号不能直接启用', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('源账号')
    expect(wrapper.text()).toContain('待验证')
    expect(wrapper.text()).not.toContain('cookie')

    const enableButton = findButton(wrapper, '启用')
    expect(enableButton.attributes('disabled')).toBeDefined()
  })

  it('创建播放账号后清空敏感表单并刷新列表', async () => {
    vi.mocked(getP115Accounts)
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({ data: [account({ id: 'created', role: 'playback', alias: '播放小号' })] })

    const wrapper = mountView()
    await flushPromises()
    await findButton(wrapper, '添加账号').trigger('click')

    const dialog = wrapper.get('[data-test="form-dialog"]')
    expect(dialog.attributes('data-title')).toBe('添加 115 账号')
    await dialog.get('select').setValue('playback')
    await dialog.get('input[placeholder="例如：播放小号"]').setValue('播放小号')
    await dialog.get('input[placeholder="例如：web"]').setValue('web')
    await dialog.get('input[placeholder="请输入固定的 User-Agent"]').setValue('Ember Test')
    await dialog.get('input[placeholder="请输入播放小号目标目录 ID"]').setValue('target-1')
    await dialog.get('input[placeholder="粘贴完整 Cookie"]').setValue('UID=100_A1')
    await findButton(wrapper, '保存账号').trigger('click')
    await flushPromises()

    expect(createP115Account).toHaveBeenCalledWith({
      role: 'playback',
      alias: '播放小号',
      cookie: 'UID=100_A1',
      appType: 'web',
      userAgent: 'Ember Test',
      targetParentId: 'target-1',
    })
    expect(getP115Accounts).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="form-dialog"]').exists()).toBe(false)
    expect(ElMessage.success).toHaveBeenCalledWith('115 账号已添加')
  })

  it('验证成功后刷新状态并允许启用', async () => {
    vi.mocked(getP115Accounts)
      .mockResolvedValueOnce({ data: [account()] })
      .mockResolvedValueOnce({ data: [account({ status: 'active', providerUserId: '100' })] })
      .mockResolvedValueOnce({ data: [account({ status: 'active', providerUserId: '100', enabled: true })] })

    const wrapper = mountView()
    await flushPromises()
    await findButton(wrapper, '验证').trigger('click')
    await flushPromises()

    expect(validateP115Account).toHaveBeenCalledWith('p115_source')
    expect(findButton(wrapper, '启用').attributes('disabled')).toBeUndefined()

    await findButton(wrapper, '启用').trigger('click')
    await flushPromises()
    expect(setP115AccountEnabled).toHaveBeenCalledWith('p115_source', true)
  })

  it('验证请求失败后仍刷新后端已经落库的 error 状态', async () => {
    vi.mocked(validateP115Account).mockRejectedValueOnce(new Error('provider unavailable'))
    vi.mocked(getP115Accounts)
      .mockResolvedValueOnce({ data: [account({ status: 'active', enabled: true })] })
      .mockResolvedValueOnce({
        data: [account({
          status: 'error',
          enabled: true,
          lastErrorCode: 'provider_unavailable',
          lastErrorMessage: '115 服务暂不可用',
        })],
      })

    const wrapper = mountView()
    await flushPromises()
    await findButton(wrapper, '验证').trigger('click')
    await flushPromises()

    expect(getP115Accounts).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('验证异常')
    expect(wrapper.text()).toContain('115 服务暂不可用')
  })

  it('多个账号并发刷新时丢弃先发请求的迟到响应', async () => {
    let resolveSlowRequest!: (value: { data: P115Account[] }) => void
    const slowRequest = new Promise<{ data: P115Account[] }>((resolve) => {
      resolveSlowRequest = resolve
    })
    const accountA = account({ id: 'account_a', alias: '账号 A', status: 'active' })
    const accountB = account({ id: 'account_b', alias: '账号 B', status: 'active' })
    vi.mocked(getP115Accounts)
      .mockResolvedValueOnce({ data: [accountA, accountB] })
      .mockReturnValueOnce(slowRequest)
      .mockResolvedValueOnce({ data: [accountA, account({ ...accountB, alias: '账号 B 最新状态' })] })

    const wrapper = mountView()
    await flushPromises()
    const validateButtons = wrapper.findAll('button').filter(button => button.text() === '验证')
    expect(validateButtons).toHaveLength(2)

    await validateButtons[0].trigger('click')
    await flushPromises()
    await validateButtons[1].trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('账号 B 最新状态')

    resolveSlowRequest({ data: [account({ ...accountA, alias: '过期响应' }), accountB] })
    await flushPromises()
    expect(wrapper.text()).toContain('账号 B 最新状态')
    expect(wrapper.text()).not.toContain('过期响应')
  })

  it('替换 Cookie 时不回填旧值，成功后关闭并刷新 pending 状态', async () => {
    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '替换 Cookie').trigger('click')
    const dialog = wrapper.get('[data-test="form-dialog"]')
    expect(dialog.attributes('data-title')).toBe('替换 Cookie')
    const cookieInput = dialog.get('input[placeholder="粘贴新的完整 Cookie"]')
    expect((cookieInput.element as HTMLInputElement).value).toBe('')
    await cookieInput.setValue('UID=200_A1')
    await findButton(wrapper, '确认替换').trigger('click')
    await flushPromises()

    expect(replaceP115AccountCookie).toHaveBeenCalledWith('p115_source', 'UID=200_A1')
    expect(getP115Accounts).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-test="form-dialog"]').exists()).toBe(false)
    expect(ElMessage.success).toHaveBeenCalledWith('Cookie 已替换，请重新验证账号')
  })
})
