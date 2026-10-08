import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserEntitlementsPanel from './UserEntitlementsPanel.vue'

const api = vi.hoisted(() => ({ getEntitlements: vi.fn(), adjustEntitlement: vi.fn() }))
vi.mock('@/api/entitlements', () => api)
vi.mock('element-plus', () => ({ ElMessage: { warning: vi.fn(), success: vi.fn() }, ElMessageBox: { confirm: vi.fn().mockResolvedValue(true) } }))

const selectStub = defineComponent({ props: ['modelValue'], emits: ['update:modelValue'], template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><slot /></select>' })
const optionStub = defineComponent({ props: ['value', 'label'], template: '<option :value="value">{{ label }}</option>' })

/** 使用原生表单 stub 验证用户操作，不请求后端。 */
function mountPanel(admin = false) {
  return mount(UserEntitlementsPanel, { props: admin ? { userId: 'user-1', groups: [{ key: 'B', name: '全资源', isDefault: false, sortOrder: 0, p115PlaybackMode: 'system', p115TransferHourlyLimit: 5, p115TransferDailyLimit: 10 }] } : {}, global: {
    directives: { loading: () => {} },
    stubs: { ElSelect: selectStub, ElOption: optionStub, ElForm: { template: '<form><slot /></form>' }, ElFormItem: { template: '<div><slot /></div>' }, ElInputNumber: true, ElDatePicker: true, ElTag: { template: '<span><slot /></span>' } }
  } })
}

describe('用户权益管理', () => {
  beforeEach(() => { vi.clearAllMocks(); api.getEntitlements.mockResolvedValue({ data: [], businessTimezone: 'Asia/Shanghai' }); api.adjustEntitlement.mockResolvedValue(undefined) })
  it('仅为开启保号的权益展示要求，失效权益不显示永久有效', async () => {
    api.getEntitlements.mockResolvedValue({businessTimezone:'Asia/Shanghai',data:[
      {userId:'user-1',planGroup:'A',planGroupName:'A',validityType:'permanent',expiresAt:null,watchRetentionInvalidatedAt:'2026-10-01T00:00:00Z',watchRetention:{days:14,minMinutes:90,state:'invalidated'}},
      {userId:'user-1',planGroup:'B',planGroupName:'B',validityType:'duration',expiresAt:'2099-10-01T00:00:00Z'},
    ]})
    const wrapper=mountPanel();await flushPromises()
    expect(wrapper.text().match(/观看要求：/g)).toHaveLength(1)
    expect(wrapper.text()).toContain('最近 14 天累计观看至少 90 分钟')
    expect(wrapper.text()).toContain('权益已失效')
    expect(wrapper.text()).not.toContain('永久有效')
    wrapper.unmount()
  })
  it('没有权益时不显示永久，也不提供用户自行切组入口', async () => {
    const wrapper = mountPanel()
    await flushPromises()
    expect(wrapper.text()).toContain('暂无权益')
    expect(wrapper.text()).not.toContain('永久')
    expect(wrapper.find('form').exists()).toBe(false)
  })
  it('管理员只延长选定分组，失败重试沿用操作号', async () => {
    api.adjustEntitlement.mockRejectedValueOnce(new Error('response lost'))
    const wrapper = mountPanel(true)
    await flushPromises()
    await wrapper.find('select').setValue('B')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.adjustEntitlement).toHaveBeenCalledTimes(2)
    const first = api.adjustEntitlement.mock.calls[0]!
    expect(first[0]).toBe('user-1')
    expect(first[1]).toMatchObject({ planGroup: 'B', action: 'extend', days: 30 })
    expect(api.adjustEntitlement.mock.calls[1]![1].operationId).toBe(first[1].operationId)
  })

it('三种操作直接可见，设永久只发送永久所需字段', async () => {
 api.getEntitlements.mockResolvedValue({ data: [], businessTimezone: 'Asia/Shanghai' })
 const wrapper = mountPanel(true)
 await flushPromises()
 expect(wrapper.text()).toContain('延长天数')
 expect(wrapper.text()).toContain('指定到期时间')
 expect(wrapper.text()).toContain('设为永久')
 await wrapper.find('select').setValue('B')
 await wrapper.findAll('[role="radio"]').find(button => button.text() === '设为永久')!.trigger('click')
 await wrapper.find('form').trigger('submit'); await flushPromises()
 expect(api.adjustEntitlement).toHaveBeenLastCalledWith('user-1', { operationId: expect.any(String), planGroup: 'B', action: 'set', validityType: 'permanent' })
 wrapper.unmount()
})


it('指定到期时间提交业务时间，缺失日期时不提交；取消不触发写入', async () => {
 const wrapper = mountPanel(true)
 await flushPromises()
 await wrapper.find('select').setValue('B')
 await wrapper.findAll('[role="radio"]').find(button => button.text() === '指定到期时间')!.trigger('click')
 await wrapper.find('form').trigger('submit'); await flushPromises()
 expect(api.adjustEntitlement).not.toHaveBeenCalled()
 wrapper.findComponent({ name: 'ElDatePicker' }).vm.$emit('update:modelValue', '2099-10-05 12:30:00')
 await wrapper.find('form').trigger('submit'); await flushPromises()
 expect(api.adjustEntitlement).toHaveBeenCalledWith('user-1', { operationId: expect.any(String), planGroup: 'B', action: 'set', validityType: 'duration', expiresAt: '2099-10-05 12:30:00' })
 await wrapper.findAll('button').find(button => button.text() === '取消')!.trigger('click')
 expect(wrapper.emitted('close')).toHaveLength(1)
 expect(api.adjustEntitlement).toHaveBeenCalledTimes(1)
 wrapper.unmount()
})
})
