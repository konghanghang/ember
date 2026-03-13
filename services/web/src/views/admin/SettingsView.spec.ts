import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import SettingsView from './SettingsView.vue'
import type { AdminConfigItem, ConfigGroupTestResult } from '@/types/api'
import {
  getConfigs,
  importConfigEnv,
  runCronJob,
  testConfigGroup,
  updateConfig,
} from '@/api/admin'
import { ElMessage, ElMessageBox } from 'element-plus'

vi.mock('@/api/admin', () => ({
  getConfigs: vi.fn(),
  importConfigEnv: vi.fn(),
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
  ElMessageBox: {
    confirm: vi.fn(),
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
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        value: props.modelValue,
        placeholder: props.placeholder,
        onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLInputElement).value),
      })
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
    description: 'Telegram 欢迎消息中展示的群组链接，留空表示关闭',
    type: 'url',
    placeholder: 'https://t.me/ember',
    editable: true,
    sensitive: false,
    restartRequired: false,
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
    vi.mocked(importConfigEnv).mockResolvedValue({ imported: [], skipped: {}, failed: {} })
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
    expect(wrapper.text()).toContain('https://t.me/updated')
  })

  it('可清空数据库覆盖值并立即刷新当前项状态', async () => {
    vi.mocked(getConfigs).mockResolvedValue({
      data: [
        createConfigItem({
          source: 'database',
          value: 'https://t.me/database',
        }),
      ],
    })
    vi.mocked(ElMessageBox.confirm).mockResolvedValue('confirm')
    vi.mocked(updateConfig).mockResolvedValue(
      createConfigItem({
        source: 'default',
        hasValue: false,
        value: '',
      })
    )

    const wrapper = mountView()
    await flushPromises()

    await findButton(wrapper, '移除数据库覆盖值').trigger('click')
    await flushPromises()

    expect(ElMessageBox.confirm).toHaveBeenCalled()
    expect(updateConfig).toHaveBeenCalledWith('notify_group_link', { clear: true })
    expect(ElMessage.success).toHaveBeenCalledWith('通知群组链接已移除数据库覆盖值')
    expect(wrapper.text()).toContain('未设置')
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
})
