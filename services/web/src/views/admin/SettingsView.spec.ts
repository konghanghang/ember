import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsView from './SettingsView.vue'
import type { AdminConfigItem, ConfigGroupTestResult } from '@/types/api'
import {
  getConfigs,
  getExternalApiKeyStatus,
  runCronJob,
  testConfigGroup,
  updateConfig,
} from '@/api/admin'
import { ElMessage } from 'element-plus'

vi.mock('@/api/admin', () => ({
  getConfigs: vi.fn(),
  getExternalApiKeyStatus: vi.fn(),
  runCronJob: vi.fn(),
  testConfigGroup: vi.fn(),
  updateConfig: vi.fn(),
}))

vi.mock('element-plus', () => ({
  ElMessage: {
    info: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}))

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

const ElInputStub = defineComponent({
  props: {
    modelValue: {
      type: String,
      default: '',
    },
    placeholder: {
      type: String,
      default: '',
    },
    type: {
      type: String,
      default: 'text',
    },
    rows: {
      type: Number,
      default: 2,
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => {
      if (props.type === 'textarea') {
        return h('textarea', {
          value: props.modelValue,
          placeholder: props.placeholder,
          rows: props.rows,
          onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLTextAreaElement).value),
        })
      }

      return h('input', {
        value: props.modelValue,
        placeholder: props.placeholder,
        onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
      })
    }
  },
})

const ElInputNumberStub = defineComponent({
  props: {
    modelValue: {
      type: Number,
      default: 0,
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        type: 'number',
        value: props.modelValue,
        onInput: (event: Event) =>
          emit('update:modelValue', Number((event.target as HTMLInputElement).value || 0)),
      })
  },
})

const ElSwitchStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) =>
          emit('update:modelValue', (event.target as HTMLInputElement).checked),
      })
  },
})

function createConfigItem(overrides: Partial<AdminConfigItem> = {}): AdminConfigItem {
  return {
    key: 'notify_group_link',
    group: 'business',
    groupLabel: '基础业务',
    label: '通知群组链接',
    description: 'Telegram 欢迎消息中展示的群组链接',
    type: 'url',
    placeholder: 'https://t.me/ember',
    multiline: false,
    editable: true,
    sensitive: false,
    restartRequired: false,
    allowEmpty: true,
    emptyValueMode: 'disable',
    emptyValueHint: '保存为空值后将关闭欢迎消息中的群组链接展示。',
    missingValueLevel: 'none',
    source: 'env',
    hasValue: true,
    value: 'https://t.me/original',
    ...overrides,
  }
}

function mountView() {
  return mount(SettingsView, {
    global: {
      directives: {
        loading: {
          mounted() {},
          updated() {},
        },
      },
      stubs: {
        'el-icon': passthroughStub,
        'el-tooltip': passthroughStub,
        'el-input': ElInputStub,
        'el-input-number': ElInputNumberStub,
        'el-switch': ElSwitchStub,
        'el-checkbox-group': passthroughStub,
        'el-checkbox': passthroughStub,
        'el-radio-group': passthroughStub,
        'el-radio-button': passthroughStub,
      },
    },
  })
}

function findButton(wrapper: ReturnType<typeof mountView>, label: string) {
  const button = wrapper.findAll('button').find(item => item.text().includes(label))
  expect(button, `未找到按钮: ${label}`).toBeTruthy()
  return button!
}

describe('SettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getExternalApiKeyStatus).mockResolvedValue({ data: { configured: false } } as never)
    vi.mocked(runCronJob).mockResolvedValue({ message: '任务执行成功' } as never)
  })

  it('挂载后可保存当前分组配置变更', async () => {
    vi.mocked(getConfigs)
      .mockResolvedValueOnce({
        data: [createConfigItem()],
      })
      .mockResolvedValueOnce({
        data: [
          createConfigItem({
            source: 'database',
            value: 'https://t.me/updated',
          }),
        ],
      })
    vi.mocked(updateConfig).mockResolvedValue(
      createConfigItem({
        source: 'database',
        value: 'https://t.me/updated',
      })
    )

    const wrapper = mountView()
    await flushPromises()

    const input = wrapper.find('input[placeholder="https://t.me/ember"]')
    expect(input.exists()).toBe(true)
    await input.setValue('https://t.me/updated')
    await flushPromises()

    await findButton(wrapper, '保存本组配置').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith('notify_group_link', {
      value: 'https://t.me/updated',
    })
    expect(getConfigs).toHaveBeenCalledTimes(2)
    expect(ElMessage.success).toHaveBeenCalledWith('基础业务保存成功')
    expect((wrapper.find('input[placeholder="https://t.me/ember"]').element as HTMLInputElement).value).toBe('https://t.me/updated')
  })

  it('Gateway Emby 网页开关作为后台配置保存并标记立即生效', async () => {
    const webConfig = createConfigItem({
      key: 'PLAYBACK_GATEWAY_WEB_ENABLED',
      group: 'media',
      groupLabel: '媒体集成',
      label: 'Gateway Emby 网页访问',
      description: '控制是否允许通过 Playback Gateway 打开 Emby 网页端',
      type: 'boolean',
      source: 'default',
      value: 'true',
      restartRequired: false,
      allowEmpty: false,
      emptyValueMode: 'not_allowed',
    })
    vi.mocked(getConfigs)
      .mockResolvedValueOnce({ data: [webConfig] })
      .mockResolvedValueOnce({
        data: [createConfigItem({ ...webConfig, source: 'database', value: 'false' })],
      })
    vi.mocked(updateConfig).mockResolvedValue(
      createConfigItem({ ...webConfig, source: 'database', value: 'false' })
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Gateway Emby 网页访问')
    expect(wrapper.text()).toContain('立即生效')
    const toggle = wrapper.find('input[type="checkbox"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)
    await toggle.setValue(false)
    await flushPromises()

    await findButton(wrapper, '保存本组配置').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith('PLAYBACK_GATEWAY_WEB_ENABLED', { value: 'false' })
    expect(ElMessage.success).toHaveBeenCalledWith('媒体集成保存成功')
  })

  it('多行配置项会渲染为 textarea 并按字符串保存', async () => {
    vi.mocked(getConfigs)
      .mockResolvedValueOnce({
        data: [
          createConfigItem({
            key: 'telegram_welcome_message_template',
            label: 'Telegram 欢迎语模板',
            description: 'Telegram 入群欢迎语模板，支持 {names} 和 {notifyGroupLink} 占位符',
            multiline: true,
            value: '👋 欢迎 <b>{names}</b> 加入！',
          }),
        ],
      })
      .mockResolvedValueOnce({
        data: [
          createConfigItem({
            key: 'telegram_welcome_message_template',
            label: 'Telegram 欢迎语模板',
            description: 'Telegram 入群欢迎语模板，支持 {names} 和 {notifyGroupLink} 占位符',
            multiline: true,
            source: 'database',
            value: '欢迎 {names}\n通知群：{notifyGroupLink}',
          }),
        ],
      })
    vi.mocked(updateConfig).mockResolvedValue(
      createConfigItem({
        key: 'telegram_welcome_message_template',
        label: 'Telegram 欢迎语模板',
        description: 'Telegram 入群欢迎语模板，支持 {names} 和 {notifyGroupLink} 占位符',
        multiline: true,
        source: 'database',
        value: '欢迎 {names}\n通知群：{notifyGroupLink}',
      })
    )

    const wrapper = mountView()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    expect(textarea.exists()).toBe(true)
    await textarea.setValue('欢迎 {names}\n通知群：{notifyGroupLink}')
    await flushPromises()

    await findButton(wrapper, '保存本组配置').trigger('click')
    await flushPromises()

    expect(updateConfig).toHaveBeenCalledWith('telegram_welcome_message_template', {
      value: '欢迎 {names}\n通知群：{notifyGroupLink}',
    })
  })

  it('会把数据库显式空值与普通未设置区分展示', async () => {
    vi.mocked(getConfigs).mockResolvedValue({
      data: [
        createConfigItem({
          source: 'database',
          hasValue: false,
          value: '',
        }),
      ],
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('数据库显式空值')
    expect(wrapper.text()).not.toContain('保存为空值后将关闭欢迎消息中的群组链接展示。')
    expect(wrapper.find('button[aria-label="查看通知群组链接说明"]').exists()).toBe(true)
  })

  it('切换到邮件服务分组后可触发测试连接并展示聚合失败信息', async () => {
    const emailItem = createConfigItem({
      key: 'SMTP_HOST',
      group: 'email',
      groupLabel: '邮件服务',
      label: 'SMTP 主机',
      description: '邮件服务器主机地址',
      placeholder: 'smtp.example.com',
      value: 'smtp.example.com',
      source: 'database',
    })

    vi.mocked(getConfigs).mockResolvedValue({
      data: [createConfigItem(), emailItem],
    })
    vi.mocked(testConfigGroup).mockResolvedValue({
      success: false,
      message: '邮件配置检查失败',
      details: [
        {
          target: 'smtp',
          success: false,
          message: '连接失败',
        },
      ],
    } satisfies ConfigGroupTestResult)

    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '邮件服务').trigger('click')
    await flushPromises()
    await findButton(wrapper, '测试连接').trigger('click')
    await flushPromises()

    expect(testConfigGroup).toHaveBeenCalledWith('email')
    expect(ElMessage.warning).toHaveBeenCalledWith('smtp: 连接失败')
  })

  it('只读边界项会展示只读原因和缺失影响', async () => {
    vi.mocked(getConfigs).mockResolvedValue({
      data: [
        createConfigItem({
          key: 'TELEGRAM_BOT_TOKEN',
          group: 'deployment',
          groupLabel: '部署与密钥',
          label: 'Telegram Bot Token',
          description: 'Telegram Bot 令牌',
          type: 'secret',
          editable: false,
          sensitive: true,
          restartRequired: true,
          source: 'unset',
          hasValue: false,
          value: undefined,
          allowEmpty: false,
          emptyValueMode: 'not_allowed',
          readOnlyHint: 'Bot 启动时必须从部署环境读取该令牌；修改后需要重启 Bot，不应在后台在线编辑。',
          missingValueHint: '未设置时 Telegram Bot 无法启动，也无法接收或发送通知。',
          missingValueLevel: 'critical',
        }),
      ],
    })

    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '部署与密钥').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('只读')
    expect(wrapper.text()).toContain('高风险缺失')
    expect(wrapper.text()).not.toContain('Bot 启动时必须从部署环境读取该令牌')
    expect(wrapper.find('button[aria-label="查看Telegram Bot Token说明"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('仅展示当前状态')
    expect(wrapper.text()).toContain('未设置时 Telegram Bot 无法启动，也无法接收或发送通知。')
    expect(wrapper.text()).toContain('当前分组缺少 Telegram Bot Token，这些项通常需要通过部署环境补齐。')
  })
})
