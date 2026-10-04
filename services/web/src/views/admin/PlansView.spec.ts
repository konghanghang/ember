import { defineComponent, h } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PlansView from './PlansView.vue'
import PlanBenefitsEditor from '@/components/billing/PlanBenefitsEditor.vue'
import { createPlan, getPlanGroups, getPlans, updatePlan } from '@/api/admin'
import type { Plan } from '@/types/api'

vi.mock('@/api/admin', () => ({ createPlan: vi.fn(), deletePlan: vi.fn(), getPlanGroups: vi.fn(), getPlans: vi.fn(), updatePlan: vi.fn() }))
vi.mock('element-plus', () => ({ ElMessage: { success: vi.fn(), warning: vi.fn() }, ElMessageBox: { confirm: vi.fn() } }))
const passthrough = defineComponent({ setup(_, { slots }) { return () => h('div', slots.default?.()) } })

/** Mount the real page with fake API and form containers, without backend or payment traffic. */
async function mountView() {
 const wrapper = shallowMount(PlansView, { global: { directives: { loading: () => {} }, stubs: {
 EmberFormDialog: passthrough, EmberTableCard: passthrough, EmberPageHeaderCard: passthrough,
 'el-form': passthrough, 'el-form-item': passthrough,
 ...Object.fromEntries(['icon','option','switch','table-column','tag','pagination','input','input-number','select'].map(name => [`el-${name}`, true]))
 } } })
 await flushPromises()
 return wrapper
}

type Form = { name: string; planGroup: string; validityType: 'duration' | 'permanent'; days: number }
type Page = { form: Form; editForm: Form; handleCreate: () => Promise<void>; handleUpdate: () => Promise<void>; openEditDialog: (plan: Plan) => void }

describe('单分组套餐管理', () => {
 beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(getPlanGroups).mockResolvedValue({ data: [{ key: 'BASE', name: '基础组', isDefault: true, sortOrder: 0, p115PlaybackMode: 'system', p115TransferHourlyLimit: 5, p115TransferDailyLimit: 10 }] })
  vi.mocked(getPlans).mockResolvedValue({ data: [], total: 0, page: 1, pageSize: 10, totalPages: 0 })
 })
 it('默认选组并只提交一组限时字段，不展示组合编辑器', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.form.name = '增强月卡'
  await vm.handleCreate()
  expect(createPlan).toHaveBeenCalledWith(expect.objectContaining({ planGroup: 'BASE', validityType: 'duration', days: 30 }))
  expect(vi.mocked(createPlan).mock.calls[0]?.[0]).not.toHaveProperty('benefits')
  expect(w.findComponent(PlanBenefitsEditor).exists()).toBe(false)
  w.unmount()
 })
 it('永久商品归一化天数为0，不提交残留限时天数', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.form.name='基础永久'; vm.form.validityType='permanent'; vm.form.days=180
  await vm.handleCreate()
  expect(createPlan).toHaveBeenCalledWith(expect.objectContaining({ validityType:'permanent', days:0 }))
  w.unmount()
 })
 it('编辑时读取单组字段，永久转限时后提交明确天数', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.openEditDialog({ id:'p1', name:'基础永久', description:'', planGroup:'BASE', validityType:'permanent', days:0, price:999, currency:'usd', isActive:true, sortOrder:0, createdAt:'', updatedAt:'' } as Plan)
  expect(vm.editForm.validityType).toBe('permanent')
  vm.editForm.validityType='duration'; vm.editForm.days=60
  await vm.handleUpdate()
  expect(updatePlan).toHaveBeenCalledWith('p1',expect.objectContaining({ planGroup:'BASE', validityType:'duration', days:60 }))
  expect(vi.mocked(updatePlan).mock.calls[0]?.[1]).not.toHaveProperty('benefits')
  w.unmount()
 })
 it('拒绝未选分组和0天限时商品', async () => {
  const w = await mountView(); const vm = w.vm as unknown as Page
  vm.form.name='无效'; vm.form.days=0
  await vm.handleCreate()
  expect(createPlan).not.toHaveBeenCalled()
  vm.form.days=30; vm.form.planGroup=''
  await vm.handleCreate()
  expect(createPlan).not.toHaveBeenCalled()
  w.unmount()
 })
})
