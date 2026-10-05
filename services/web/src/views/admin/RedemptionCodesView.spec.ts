import { defineComponent, h } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RedemptionCodesView from './RedemptionCodesView.vue'
import { createRedemptionCode, createRedemptionCodesBatch, getPlanGroups, getRedemptionCodes, updateRedemptionCode } from '@/api/admin'
import type { RedemptionCode } from '@/types/api'

vi.mock('@/api/admin', () => ({ createRedemptionCode: vi.fn(), createRedemptionCodesBatch: vi.fn(), updateRedemptionCode: vi.fn(), deleteRedemptionCode: vi.fn(), getPlanGroups: vi.fn(), getRedemptionCodes: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn(), warning: vi.fn() }, ElMessageBox: { confirm: vi.fn() } }))
const passthrough = defineComponent({ setup(_, { slots }) { return () => h('div', slots.default?.()) } })
type Form = { count: number; validityType: 'duration' | 'permanent'; defaultDays: number; registrationPlanGroup: string }
type Page = { form: Form; editForm: Form; openCreateDialog: () => void; openEditDialog: (row: RedemptionCode) => void; handleCreate: () => Promise<void>; handleUpdate: () => Promise<void> }

/** Exercise the real page submission with API fakes and lightweight form containers. */
async function mountView() {
 const wrapper = shallowMount(RedemptionCodesView, { global: { directives: { loading: () => {} }, stubs: {
 EmberFormDialog: passthrough, EmberTableCard: passthrough, EmberPageHeaderCard: passthrough,
 'el-form': passthrough, 'el-form-item': passthrough,
 ...Object.fromEntries(['icon','option','switch','table-column','tag','pagination','input','input-number','select','date-picker','progress'].map(name => [`el-${name}`, true]))
 } } })
 await flushPromises()
 return wrapper
}

describe('兑换码权益类型', () => {
 beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getPlanGroups).mockResolvedValue({ data: [{ key: 'BASE', name: '基础组', isDefault: true }] } as never)
  vi.mocked(getRedemptionCodes).mockResolvedValue({ data: [], total: 0, page: 1, pageSize: 10, totalPages: 0 })
  vi.mocked(createRedemptionCodesBatch).mockResolvedValue({ data: [], count: 2 })
 })
 it('默认按天并保留30天，永久类型清除请求中的残留天数', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.openCreateDialog()
  await vm.handleCreate()
  expect(createRedemptionCode).toHaveBeenCalledWith(expect.objectContaining({ validityType: 'duration', defaultDays: 30 }))
  vm.form.validityType = 'permanent'; vm.form.defaultDays = 90
  await vm.handleCreate()
  expect(createRedemptionCode).toHaveBeenLastCalledWith(expect.objectContaining({ validityType: 'permanent', defaultDays: 0, registrationPlanGroup: 'BASE' }))
  w.unmount()
 })
 it('批量永久码同样提交永久权益，按天不接受0天', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.openCreateDialog(); vm.form.defaultDays = 0
  await vm.handleCreate()
  expect(createRedemptionCode).not.toHaveBeenCalled()
  vm.form.count = 2; vm.form.validityType = 'permanent'
  await vm.handleCreate()
  expect(createRedemptionCodesBatch).toHaveBeenCalledWith(expect.objectContaining({ count: 2, validityType: 'permanent', defaultDays: 0 }))
  w.unmount()
 })
 it('编辑读取永久类型并支持改为按天，不混淆兑换截止时间', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.openEditDialog({ id: 'code1', maxUses: 2, usedCount: 0, validityType: 'permanent', defaultDays: 0, registrationPlanGroup: 'BASE', expiresAt: null } as RedemptionCode)
  expect(vm.editForm.validityType).toBe('permanent')
  await vm.handleUpdate()
  expect(updateRedemptionCode).toHaveBeenCalledWith('code1', expect.objectContaining({ validityType: 'permanent', defaultDays: 0, expiresAt: null }))
  vm.editForm.validityType = 'duration'; vm.editForm.defaultDays = 60
  await vm.handleUpdate()
  expect(updateRedemptionCode).toHaveBeenLastCalledWith('code1', expect.objectContaining({ validityType: 'duration', defaultDays: 60 }))
  w.unmount()
 })
})
