import { defineComponent, h, reactive } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AccountCenterView from './AccountCenterView.vue'
import {
  applyCurrentUserMediaLibraryPolicySync,
  getUserMediaLibraries,
  resetUserMediaLibraryPreferences,
  sendEmailChangeCode,
  updateUserMediaLibraries,
  updatePassword
} from '@/api/console'
import { bindAdminEmbyAccount, getAdminEmbyUsers } from '@/api/admin'
import { ElMessage, ElMessageBox } from 'element-plus'

vi.mock('@/api/console', () => ({
  applyCurrentUserMediaLibraryPolicySync: vi.fn(),
  sendEmailChangeCode: vi.fn(),
  generateTelegramBindCode: vi.fn(),
  getUserMediaLibraries: vi.fn(),
  resetUserMediaLibraryPreferences: vi.fn(),
  unbindTelegram: vi.fn(),
  updateUserMediaLibraries: vi.fn(),
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
  ElMessageBox: {
    confirm: vi.fn(),
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
    passwordResetRequired: false as boolean | undefined,
  },
})
const authStoreState = reactive({
  isAdmin: false,
  passwordResetRequired: false,
})

const updateEmailMock = vi.fn(async (newEmail: string, _code: string) => {
  userStoreState.profile.email = newEmail
})
const updatePasswordMock = vi.fn(async (_oldPassword: string, _newPassword: string) => {
  userStoreState.profile.passwordResetRequired = false
  authStoreState.passwordResetRequired = false
})
const setProfileMock = vi.fn()
const fetchProfileMock = vi.fn()

vi.mock('@/store/user', () => ({
  useUserStore: vi.fn(() => ({
    get profile() {
      return userStoreState.profile
    },
    updateEmail: updateEmailMock,
    updatePassword: updatePasswordMock,
    setProfile: setProfileMock,
    fetchProfile: fetchProfileMock,
  })),
}))

vi.mock('@/store/auth', () => ({
  useAuthStore: vi.fn(() => ({
    get isAdmin() {
      return authStoreState.isAdmin
    },
    get passwordResetRequired() {
      return authStoreState.passwordResetRequired
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

const ElCheckboxStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
    disabled: {
      type: Boolean,
      default: false,
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, slots }) {
    return () => h('label', [
      h('input', {
        type: 'checkbox',
        checked: props.modelValue,
        disabled: props.disabled,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLInputElement).checked)
        },
      }),
      slots.default?.(),
    ])
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

// 视图在测试中被直接驱动的 setup 内部成员（ref 已解包）。
type AccountCenterSetupState = {
  selectedMediaLibraryIds: string[]
}

/**
 * 读取组件 setup 内部状态。
 * Vue 3.5 起 setupState 不再出现在公开的 ComponentInternalInstance 类型上（运行时仍存在），
 * 测试需要直接驱动组件内部状态，这里以结构化类型收窄出实际用到的成员。
 */
function setupStateOf(wrapper: VueWrapper): AccountCenterSetupState {
  return (wrapper.vm.$ as unknown as { setupState: AccountCenterSetupState }).setupState
}

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
        'el-checkbox': ElCheckboxStub,
        'el-input': ElInputStub,
        EmberFormDialog: EmberFormDialogStub,
        EmberEmptyStateCard: passthroughStub,
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

/** 切换账号中心分段（EmberSegmentTabs 使用 role=radio）。 */
async function switchAccountTab(wrapper: ReturnType<typeof mountView>, label: string) {
  const tab = wrapper
    .findAll('[role="radio"]')
    .find(item => item.text().includes(label))
  expect(tab, `未找到分段: ${label}`).toBeTruthy()
  await tab!.trigger('click')
  await flushPromises()
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
    vi.mocked(getUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'synced',
        libraries: [
          {
            id: 'lib_movie',
            name: '电影',
            type: 'Movie',
            itemCount: 100,
            inGroupTemplate: true,
            enabled: true,
          },
          {
            id: 'lib_series',
            name: '剧集',
            type: 'Series',
            itemCount: 200,
            inGroupTemplate: true,
            enabled: true,
          },
        ],
      },
    })
    vi.mocked(updateUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'synced',
        libraries: [
          {
            id: 'lib_movie',
            name: '电影',
            type: 'Movie',
            itemCount: 100,
            inGroupTemplate: true,
            enabled: true,
          },
          {
            id: 'lib_series',
            name: '剧集',
            type: 'Series',
            itemCount: 200,
            inGroupTemplate: true,
            enabled: true,
          },
        ],
      },
    })
    vi.mocked(resetUserMediaLibraryPreferences).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
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
      passwordResetRequired: false,
    }
    authStoreState.isAdmin = false
    authStoreState.passwordResetRequired = false
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

describe('AccountCenterView 媒体库偏好', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    userStoreState.profile = {
      id: 'u1',
      username: 'demo',
      role: 'user',
      email: 'old@example.com',
      embyId: 'emby_1',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
      passwordResetRequired: false,
    }
    authStoreState.isAdmin = false
    authStoreState.passwordResetRequired = false
    vi.mocked(getUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 1,
        policySyncStatus: 'synced',
        libraries: [
          {
            id: 'lib_movie',
            name: '电影',
            type: 'Movie',
            itemCount: 100,
            inGroupTemplate: true,
            enabled: true,
          },
          {
            id: 'lib_series',
            name: '剧集',
            type: 'Series',
            itemCount: 200,
            inGroupTemplate: true,
            enabled: false,
          },
        ],
      },
    })
    vi.mocked(updateUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 1,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
    vi.mocked(resetUserMediaLibraryPreferences).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
    vi.mocked(applyCurrentUserMediaLibraryPolicySync).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 1,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
  })

  it('加载媒体库偏好并按已启用集合保存', async () => {
    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    expect(getUserMediaLibraries).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('媒体库偏好')
    expect(wrapper.text()).toContain('电影')
    expect(wrapper.text()).toContain('剧集')

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    expect(saveButton, '未找到保存偏好按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateUserMediaLibraries).toHaveBeenCalledWith(['lib_movie'])
    expect(ElMessage.success).toHaveBeenCalledWith('媒体库偏好已保存')
  })

  it('取消勾选媒体库后保存只提交仍启用的媒体库', async () => {
    vi.mocked(getUserMediaLibraries).mockResolvedValueOnce({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'synced',
        libraries: [
          {
            id: 'lib_movie',
            name: '电影',
            type: 'Movie',
            itemCount: 100,
            inGroupTemplate: true,
            enabled: true,
          },
          {
            id: 'lib_series',
            name: '剧集',
            type: 'Series',
            itemCount: 200,
            inGroupTemplate: true,
            enabled: true,
          },
        ],
      },
    })

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const seriesCheckbox = wrapper.find('[data-library-id="lib_series"] input[type="checkbox"]')
    expect(seriesCheckbox.exists(), '未找到剧集媒体库 checkbox').toBe(true)
    await seriesCheckbox.setValue(false)
    await flushPromises()

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    expect(saveButton, '未找到保存偏好按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateUserMediaLibraries).toHaveBeenCalledWith(['lib_movie'])
    expect(updateUserMediaLibraries).not.toHaveBeenCalledWith(['lib_movie', 'lib_series'])
  })

  it('保存偏好后如果 Emby 同步失败只提示本地已保存', async () => {
    vi.mocked(updateUserMediaLibraries).mockResolvedValueOnce({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 1,
        policySyncStatus: 'failed',
        libraries: [],
      },
    })

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    expect(saveButton, '未找到保存偏好按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateUserMediaLibraries).toHaveBeenCalledWith(['lib_movie'])
    expect(ElMessage.warning).toHaveBeenCalledWith('本地已保存，Emby 同步失败，请联系管理员处理')
    expect(ElMessage.success).not.toHaveBeenCalledWith('媒体库偏好已保存')
  })

  it('保存偏好后如果等待 Emby 同步则提示等待同步', async () => {
    vi.mocked(updateUserMediaLibraries).mockResolvedValueOnce({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: true,
        templateCount: 2,
        enabledCount: 1,
        policySyncStatus: 'pending',
        libraries: [],
      },
    })

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    expect(saveButton, '未找到保存偏好按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(ElMessage.info).toHaveBeenCalledWith('本地已保存，正在等待 Emby 同步')
    expect(ElMessage.success).not.toHaveBeenCalledWith('媒体库偏好已保存')
  })

  it('保存空媒体库集合前要求二次确认', async () => {
    vi.mocked(ElMessageBox.confirm).mockResolvedValueOnce('confirm' as never)

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    setupStateOf(wrapper).selectedMediaLibraryIds = []
    await flushPromises()

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    expect(saveButton, '未找到保存偏好按钮').toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      '保存后将关闭所有媒体库显示，Emby 客户端中不会保留任何已启用媒体库。确定继续吗？',
      '确认关闭全部媒体库',
      expect.objectContaining({
        confirmButtonText: '确认关闭',
        cancelButtonText: '取消',
        type: 'warning',
      }),
    )
    expect(updateUserMediaLibraries).toHaveBeenCalledWith([])
  })

  it('取消空媒体库集合确认不会提交保存', async () => {
    vi.mocked(ElMessageBox.confirm).mockRejectedValueOnce(new Error('cancel'))

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    setupStateOf(wrapper).selectedMediaLibraryIds = []
    await flushPromises()

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    await saveButton!.trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
    expect(updateUserMediaLibraries).not.toHaveBeenCalled()
  })

  it('恢复默认会调用偏好删除接口', async () => {
    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const resetButton = wrapper.findAll('button').find(item => item.text().includes('恢复默认'))
    expect(resetButton, '未找到恢复默认按钮').toBeTruthy()
    await resetButton!.trigger('click')
    await flushPromises()

    expect(resetUserMediaLibraryPreferences).toHaveBeenCalledTimes(1)
    expect(ElMessage.success).toHaveBeenCalledWith('已恢复分组默认')
  })

  it('恢复默认后如果 Emby 同步失败只提示本地已保存', async () => {
    vi.mocked(resetUserMediaLibraryPreferences).mockResolvedValueOnce({
      data: {
        userId: 'u1',
        embyId: 'emby_1',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 2,
        enabledCount: 2,
        policySyncStatus: 'failed',
        libraries: [],
      },
    })

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const resetButton = wrapper.findAll('button').find(item => item.text().includes('恢复默认'))
    expect(resetButton, '未找到恢复默认按钮').toBeTruthy()
    await resetButton!.trigger('click')
    await flushPromises()

    expect(resetUserMediaLibraryPreferences).toHaveBeenCalledTimes(1)
    expect(ElMessage.warning).toHaveBeenCalledWith('本地已保存，Emby 同步失败，请联系管理员处理')
    expect(ElMessage.success).not.toHaveBeenCalledWith('已恢复分组默认')
  })

  it('可以把当前有效媒体库设置重新同步到 Emby', async () => {
    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const applyButton = wrapper.findAll('button').find(item => item.text().includes('同步到 Emby'))
    expect(applyButton, '未找到同步到 Emby按钮').toBeTruthy()
    await applyButton!.trigger('click')
    await flushPromises()

    expect(applyCurrentUserMediaLibraryPolicySync).toHaveBeenCalledTimes(1)
    expect(ElMessage.success).toHaveBeenCalledWith('已同步到 Emby')
  })

  it('保存遇到 409 时提示媒体库权限正在同步', async () => {
    vi.mocked(updateUserMediaLibraries).mockRejectedValueOnce({ response: { status: 409 } })

    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    const saveButton = wrapper.findAll('button').find(item => item.text().includes('保存偏好'))
    await saveButton!.trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('媒体库权限正在同步，稍后再保存')
  })

  it('点击媒体库行可切换勾选，并展示未保存更改提示', async () => {
    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '媒体库偏好')

    expect(wrapper.find('[data-test="media-library-dirty"]').exists()).toBe(false)

    const seriesCard = wrapper.find('[data-library-id="lib_series"]')
    expect(seriesCard.exists(), '未找到剧集媒体库行').toBe(true)
    await seriesCard.trigger('click')
    await flushPromises()

    expect(setupStateOf(wrapper).selectedMediaLibraryIds).toEqual(['lib_movie', 'lib_series'])
    expect(wrapper.find('[data-test="media-library-dirty"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="media-library-dirty"]').text()).toContain('有未保存更改')
  })
})

describe('AccountCenterView 布局与强制改密', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    userStoreState.profile = {
      id: 'u1',
      username: 'demo',
      role: 'user',
      email: 'old@example.com',
      embyId: '',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
      passwordResetRequired: false,
    }
    authStoreState.isAdmin = false
    authStoreState.passwordResetRequired = false
    vi.mocked(getUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'u1',
        embyId: '',
        planGroup: 'VIP',
        planGroupName: 'VIP',
        customized: false,
        templateCount: 0,
        enabledCount: 0,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
  })

  it('渲染三段导航，连接绑定并入基本资料', async () => {
    const wrapper = mountView()
    await flushPromises()

    const nav = wrapper.find('[data-test="account-section-nav"]')
    expect(nav.exists()).toBe(true)
    expect(nav.text()).toContain('基本资料')
    expect(nav.text()).toContain('安全设置')
    expect(nav.text()).toContain('媒体库偏好')
    expect(nav.text()).not.toContain('连接与绑定')

    expect(wrapper.find('#account-profile').exists()).toBe(true)
    expect(wrapper.find('#account-security').exists()).toBe(true)
    expect(wrapper.find('#account-bindings').exists()).toBe(true)
    expect(wrapper.find('#account-media-libraries').exists()).toBe(true)
    // 连接区挂在基本资料分段内，默认可见
    expect(wrapper.find('[data-test="account-section-bindings"]').isVisible()).toBe(true)

    const profileTab = wrapper.findAll('[role="radio"]').find(item => item.text().includes('基本资料'))
    expect(profileTab?.attributes('aria-checked')).toBe('true')
  })

  it('强制改密时展示风险提示，并默认进入安全设置分段', async () => {
    authStoreState.passwordResetRequired = true
    userStoreState.profile.passwordResetRequired = true

    const wrapper = mountView()
    await flushPromises()

    const banner = wrapper.find('[data-test="password-reset-banner"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toContain('当前账号必须先修改密码')

    const securityTab = wrapper.findAll('[role="radio"]').find(item => item.text().includes('安全设置'))
    expect(securityTab?.attributes('aria-checked')).toBe('true')
    expect(wrapper.find('[data-test="account-section-security"]').isVisible()).toBe(true)
  })

  it('连接区并入基本资料，使用统一双列卡片且无 sky 整块强调', async () => {
    const wrapper = mountView()
    await flushPromises()
    // 默认就是基本资料分段，无需再切「连接与绑定」

    const embyCard = wrapper.find('[data-test="binding-emby"]')
    const telegramCard = wrapper.find('[data-test="binding-telegram"]')
    expect(embyCard.exists()).toBe(true)
    expect(telegramCard.exists()).toBe(true)
    expect(embyCard.classes().join(' ')).toMatch(/border-gray/)
    expect(telegramCard.classes().join(' ')).toMatch(/border-gray/)
    expect(telegramCard.html()).not.toMatch(/bg-sky/)
  })
})

describe('AccountCenterView 管理员 Emby 绑定', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    vi.mocked(getUserMediaLibraries).mockResolvedValue({
      data: {
        userId: 'admin_1',
        embyId: 'emby_admin',
        planGroup: 'ADMIN',
        planGroupName: '管理员',
        customized: false,
        templateCount: 0,
        enabledCount: 0,
        policySyncStatus: 'synced',
        libraries: [],
      },
    })
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
    authStoreState.passwordResetRequired = false
    userStoreState.profile = {
      id: 'admin_1',
      username: 'admin',
      role: 'admin',
      email: 'admin@example.com',
      embyId: '',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
      passwordResetRequired: false,
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

describe('AccountCenterView 密码修改', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchProfileMock.mockResolvedValue(undefined)
    userStoreState.profile = {
      id: 'u1',
      username: 'demo',
      role: 'user',
      email: 'old@example.com',
      embyId: 'emby_1',
      embyDisabled: false,
      isActive: true,
      createdAt: '2026-01-01T00:00:00Z',
      passwordResetRequired: false,
    }
    authStoreState.isAdmin = false
    authStoreState.passwordResetRequired = false
  })

  it('两次空串密码不再绕过校验，明确拒绝并提示', async () => {
    const wrapper = mountView()
    await flushPromises()
    await switchAccountTab(wrapper, '安全设置')

    // passwordForm 默认三字段均为空串，过去两次空串相等会绕过不一致校验
    const updateButton = wrapper.findAll('button').find(item => item.text().includes('更新密码'))
    expect(updateButton, '未找到更新密码按钮').toBeTruthy()
    await updateButton!.trigger('click')
    await flushPromises()

    expect(ElMessage.warning).toHaveBeenCalledWith('请填写完整的密码信息')
    expect(updatePasswordMock).not.toHaveBeenCalled()
    expect(updatePassword).not.toHaveBeenCalled()
  })

  it('改密成功走 userStore 并清除强制改密标记', async () => {
    authStoreState.passwordResetRequired = true
    userStoreState.profile.passwordResetRequired = true

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-test="password-reset-banner"]').exists()).toBe(true)

    const inputs = wrapper.findAll('input[type="password"]')
    expect(inputs.length).toBeGreaterThanOrEqual(3)
    await inputs[0].setValue('old-pass')
    await inputs[1].setValue('new-pass-1')
    await inputs[2].setValue('new-pass-1')

    const updateButton = wrapper.findAll('button').find(item => item.text().includes('更新密码'))
    await updateButton!.trigger('click')
    await flushPromises()

    expect(updatePasswordMock).toHaveBeenCalledWith('old-pass', 'new-pass-1')
    expect(updatePassword).not.toHaveBeenCalled()
    expect(ElMessage.success).toHaveBeenCalledWith('密码修改成功')
    expect(userStoreState.profile.passwordResetRequired).toBe(false)
    expect(authStoreState.passwordResetRequired).toBe(false)
    expect(wrapper.find('[data-test="password-reset-banner"]').exists()).toBe(false)
  })
})
