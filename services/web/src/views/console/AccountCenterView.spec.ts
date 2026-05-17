import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountCenterView from './AccountCenterView.vue'
import { sendEmailChangeCode } from '@/api/console'
import { getRegistrationMode } from '@/api/auth'
import { bindAdminEmbyAccount, getAdminEmbyUsers } from '@/api/admin'
import { ElMessage } from 'element-plus'

vi.mock('@/api/console', () => ({
  sendEmailChangeCode: vi.fn(),
  generateTelegramBindCode: vi.fn(),
  unbindTelegram: vi.fn(),
  updatePassword: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  // 默认返回空白名单 = 无注册邮箱域名限制；让现有用例完全不感知该 hook。
  // 需要测白名单生效行为的用例可在 beforeEach / it 内 vi.mocked(getRegistrationMode).mockResolvedValueOnce(...) 覆盖。
  getRegistrationMode: vi.fn(() =>
    Promise.resolve({
      mode: 'open',
      emailVerification: false,
      allowedEmailDomains: [],
    }),
  ),
}))

vi.mock('@/api/admin', () => ({
  bindAdminEmbyAccount: vi.fn(),
  getAdminEmbyUsers: vi.fn(),
  unbindAdminEmbyAccount: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push: vi.fn() }),
  }
})

vi.mock('element-plus', () => ({
  ElMessage: {
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
    error: vi.fn(),
  },
}))

const userStoreState = reactive({
  profile: {
    id: 'u1',
    username: 'demo',
    role: 'user' as 'user' | 'admin',
    email: 'old@example.com',
    embyId: '',
    embyDisabled: false,
    isActive: true,
    createdAt: '2026-01-01T00:00:00Z',
  },
})
const authStoreState = reactive({
  isAdmin: false,
})

const updateEmailMock = vi.fn(async (newEmail: string, _code: string) => {
  userStoreState.profile.email = newEmail
})
const setProfileMock = vi.fn()
const fetchProfileMock = vi.fn()

vi.mock('@/store/user', () => ({
  useUserStore: vi.fn(() => ({
    get profile() {
      return userStoreState.profile
    },
    updateEmail: updateEmailMock,
    setProfile: setProfileMock,
    fetchProfile: fetchProfileMock,
  })),
}))

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(() => ({
    get isAdmin() {
      return authStoreState.isAdmin
    },
  })),
}))

vi.mock('@/store/console', () => ({
  useConsoleStore: vi.fn(() => ({})),
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const ElInputStub = defineComponent({
  props: {
    modelValue: { type: [String, Number], default: '' },
    placeholder: { type: String, default: '' },
    type: { type: String, default: 'text' },
    maxlength: { type: [String, Number], default: undefined },
    inputmode: { type: String, default: undefined },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        type: props.type === 'password' ? 'password' : 'text',
        value: props.modelValue,
        placeholder: props.placeholder,
        maxlength: props.maxlength,
        inputmode: props.inputmode,
        onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
      })
  },
})

const EmberFormDialogStub = defineComponent({
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(props, { slots }) {
    return () => {
      if (!props.modelValue) {
        return null
      }
      return h(
        'div',
        {
          'data-test': 'verify-dialog',
          'data-title': props.title,
        },
        [
          h('div', { 'data-test': 'dialog-body' }, slots.default?.()),
          h('div', { 'data-test': 'dialog-footer' }, slots.footer?.()),
        ],
      )
    }
  },
})

function mountView() {
  return mount(AccountCenterView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        'el-icon': passthroughStub,
        'el-input': ElInputStub,
        EmberFormDialog: EmberFormDialogStub,
        DefaultAvatar: passthroughStub,
        Bell: passthroughStub,
        ChatDotRound: passthroughStub,
        Check: passthroughStub,
        CopyDocument: passthroughStub,
        Key: passthroughStub,
        Lock: passthroughStub,
        Message: passthroughStub,
        Monitor: passthroughStub,
        Reading: passthroughStub,
        UserFilled: passthroughStub,
      },
    },
  })
}

function findEmailInput(wrapper: ReturnType<typeof mountView>) {
  const input = wrapper.find('input[placeholder="name@example.com"]')
  expect(input.exists(), '未找到联系邮箱输入框').toBe(true)
  return input
}

function findSaveButton(wrapper: ReturnType<typeof mountView>) {
  const button = wrapper
    .findAll('button')
    .find(item => item.text().includes('保存邮箱') || item.text().includes('发送中'))
  expect(button, '未找到保存邮箱按钮').toBeTruthy()
  return button!
}

function findCodeInput(wrapper: ReturnType<typeof mountView>) {
  const input = wrapper.find('input[placeholder="请输入 6 位验证码"]')
  expect(input.exists(), '未找到验证码输入框').toBe(true)
  return input
}

function findFooterButton(wrapper: ReturnType<typeof mountView>, label: string) {
  const button = wrapper
    .findAll('[data-test="dialog-footer"] button')
    .find(item => item.text().includes(label))
  expect(button, `未找到弹窗按钮: ${label}`).toBeTruthy()
  return button!
}

describe('AccountCenterView 邮箱变更验证码弹窗', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    vi.mocked(bindAdminEmbyAccount).mockResolvedValue({ embyId: 'emby_1', embyUsername: 'remote_admin' })
    vi.mocked(getAdminEmbyUsers).mockResolvedValue({ data: [] })
    userStoreState.profile = {
      id: 'u1',
      username: 'demo',
      role: 'user',
      email: 'old@example.com',
      embyId: '',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
    }
    authStoreState.isAdmin = false
  })

  it('邮箱格式不合法时不发送验证码并弹 warning', async () => {
    const wrapper = mountView()
    await flushPromises()

    const input = findEmailInput(wrapper)
    await input.setValue('not-an-email')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('请输入有效的邮箱地址')
    expect(sendEmailChangeCode).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(false)
  })

  it('邮箱与当前一致时不发送验证码并弹 info', async () => {
    const wrapper = mountView()
    await flushPromises()

    const input = findEmailInput(wrapper)
    await input.setValue('old@example.com')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    expect(ElMessage.info).toHaveBeenCalledWith('邮箱未变更')
    expect(sendEmailChangeCode).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(false)
  })

  it('合法新邮箱会调用 sendEmailChangeCode 并打开验证码弹窗', async () => {
    vi.mocked(sendEmailChangeCode).mockResolvedValue({ message: '验证码已发送' })

    const wrapper = mountView()
    await flushPromises()

    const input = findEmailInput(wrapper)
    await input.setValue('new@example.com')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    expect(sendEmailChangeCode).toHaveBeenCalledWith('new@example.com')
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="verify-dialog"]').attributes('data-title')).toBe('验证邮箱变更')
    expect(wrapper.find('[data-test="dialog-body"]').text()).toContain('已向 new@example.com 发送 6 位验证码')
  })

  it('验证码长度不足 6 位时不调用 updateEmail', async () => {
    vi.mocked(sendEmailChangeCode).mockResolvedValue({ message: '验证码已发送' })

    const wrapper = mountView()
    await flushPromises()

    await findEmailInput(wrapper).setValue('new@example.com')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    await findCodeInput(wrapper).setValue('123')
    await findFooterButton(wrapper, '确认').trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('请输入 6 位验证码')
    expect(updateEmailMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(true)
  })

  it('6 位验证码点击确认后透传 (newEmail, code) 给 store 并关闭弹窗', async () => {
    vi.mocked(sendEmailChangeCode).mockResolvedValue({ message: '验证码已发送' })

    const wrapper = mountView()
    await flushPromises()

    await findEmailInput(wrapper).setValue('new@example.com')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    await findCodeInput(wrapper).setValue('654321')
    await findFooterButton(wrapper, '确认').trigger('click')
    await flushPromises()

    expect(updateEmailMock).toHaveBeenCalledWith('new@example.com', '654321')
    expect(ElMessage.success).toHaveBeenCalledWith('邮箱更新成功')
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(false)
  })

  it('点击取消会清空验证码并关闭弹窗', async () => {
    vi.mocked(sendEmailChangeCode).mockResolvedValue({ message: '验证码已发送' })

    const wrapper = mountView()
    await flushPromises()

    await findEmailInput(wrapper).setValue('new@example.com')
    await findSaveButton(wrapper).trigger('click')
    await flushPromises()

    await findCodeInput(wrapper).setValue('111111')
    await findFooterButton(wrapper, '取消').trigger('click')
    await flushPromises()

    expect(updateEmailMock).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="verify-dialog"]').exists()).toBe(false)
  })
})

describe('AccountCenterView 管理员 Emby 绑定', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    vi.mocked(bindAdminEmbyAccount).mockResolvedValue({ embyId: 'emby_free', embyUsername: 'free_remote' })
    vi.mocked(getAdminEmbyUsers).mockResolvedValue({
      data: [
        {
          embyId: 'emby_used',
          name: 'used_remote',
          hasPassword: true,
          boundUsername: 'other',
          boundToCurrent: false,
          available: false,
        },
        {
          embyId: 'emby_free',
          name: 'free_remote',
          hasPassword: true,
          boundToCurrent: false,
          available: true,
        },
      ],
    })
    authStoreState.isAdmin = true
    userStoreState.profile = {
      id: 'admin_1',
      username: 'admin',
      role: 'admin',
      email: 'admin@example.com',
      embyId: '',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
    }
  })

  it('打开绑定弹窗不会自动加载全量 Emby 用户', async () => {
    const wrapper = mountView()
    await flushPromises()

    const openButton = wrapper.findAll('button').find(item => item.text().includes('关联 Emby 账号'))
    expect(openButton, '未找到 Emby 绑定按钮').toBeTruthy()
    await openButton!.trigger('click')
    await flushPromises()

    expect(getAdminEmbyUsers).not.toHaveBeenCalled()
    expect(wrapper.find('[data-title="关联 Emby 账号"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('输入关键词后搜索 Emby 用户')
  })

  it('输入关键词搜索 Emby 用户并禁用已占用项', async () => {
    const wrapper = mountView()
    await flushPromises()

    const openButton = wrapper.findAll('button').find(item => item.text().includes('关联 Emby 账号'))
    await openButton!.trigger('click')
    await flushPromises()

    const searchInput = wrapper.find('input[placeholder="输入用户名或 ID"]')
    expect(searchInput.exists(), '未找到 Emby 用户搜索框').toBe(true)
    await searchInput.setValue('free')
    const searchButton = wrapper.findAll('[data-title="关联 Emby 账号"] button').find(item => item.text().includes('搜索'))
    expect(searchButton, '未找到 Emby 用户搜索按钮').toBeTruthy()
    await searchButton!.trigger('click')
    await flushPromises()

    expect(getAdminEmbyUsers).toHaveBeenCalledWith({ query: 'free', limit: 20 })
    expect(getAdminEmbyUsers).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('used_remote')
    expect(wrapper.text()).toContain('已绑定 other')
    expect(wrapper.text()).toContain('free_remote')
    expect(wrapper.text()).toContain('可绑定')
  })

  it('确认绑定只提交选中的 embyId 并刷新用户资料', async () => {
    const wrapper = mountView()
    await flushPromises()

    const openButton = wrapper.findAll('button').find(item => item.text().includes('关联 Emby 账号'))
    await openButton!.trigger('click')
    await flushPromises()

    const searchInput = wrapper.find('input[placeholder="输入用户名或 ID"]')
    await searchInput.setValue('free')
    const searchButton = wrapper.findAll('[data-title="关联 Emby 账号"] button').find(item => item.text().includes('搜索'))
    await searchButton!.trigger('click')
    await flushPromises()

    const freeOption = wrapper.findAll('button').find(item => item.text().includes('free_remote'))
    expect(freeOption, '未找到可绑定 Emby 用户').toBeTruthy()
    await freeOption!.trigger('click')

    const confirmButton = wrapper.findAll('[data-test="dialog-footer"] button').find(item => item.text().includes('确认关联'))
    expect(confirmButton, '未找到确认关联按钮').toBeTruthy()
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(bindAdminEmbyAccount).toHaveBeenCalledWith({ embyId: 'emby_free' })
    expect(fetchProfileMock).toHaveBeenCalledTimes(1)
    expect(ElMessage.success).toHaveBeenCalledWith('Emby 账号已关联')
  })
})
