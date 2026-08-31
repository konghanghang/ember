import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import NewSubscriptionView from './NewSubscriptionView.vue'
import { checkExistingSubscription, createSubscription } from '@/api/console'
import { searchTmdb } from '@/api/user'
import { ElMessage } from 'element-plus'

const pushMock = vi.fn()

vi.mock('@/api/user', () => ({
  getTmdbTVSeasons: vi.fn(),
  searchTmdb: vi.fn(),
}))

vi.mock('@/api/console', () => ({
  checkExistingSubscription: vi.fn(),
  createSubscription: vi.fn(),
}))

vi.mock('vue-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-router')>()
  return {
    ...actual,
    useRouter: () => ({ push: pushMock }),
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

const passthroughStub = defineComponent({
  setup(_, { slots }) {
    return () => h('div', slots.default?.())
  },
})

function mountView() {
  return mount(NewSubscriptionView, {
    global: {
      stubs: {
        EmberEmptyStateCard: passthroughStub,
        EmberFormDialog: passthroughStub,
        EmberSearchInput: passthroughStub,
        EmberPageHeaderCard: passthroughStub,
        EmberSegmentTabs: passthroughStub,
        'el-icon': passthroughStub,
        'el-input': passthroughStub,
        'el-button': passthroughStub,
        'el-tag': passthroughStub,
        'el-scrollbar': passthroughStub,
        'el-input-number': passthroughStub,
        'el-select': passthroughStub,
        'el-option': passthroughStub,
      },
    },
  })
}

describe('NewSubscriptionView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    pushMock.mockReset()
    vi.mocked(searchTmdb).mockResolvedValue({ data: [], total: 0 } as never)
    vi.mocked(checkExistingSubscription).mockResolvedValue({
      existsInLibrary: false,
      detectionFailed: false,
    } as never)
  })

  it('uses the data field from TMDB search results', async () => {
    vi.mocked(searchTmdb).mockResolvedValue({
      data: [
        {
          id: 27205,
          title: 'Inception',
          posterPath: '/poster.jpg',
          overview: 'dreams',
        },
      ],
      total: 1,
    } as never)
    const wrapper = mountView()
    const vm = wrapper.vm as unknown as {
      searchQuery: string
      handleSearch: () => Promise<void>
      results: Array<{ id: number; title: string }>
    }

    vm.searchQuery = 'Inception'
    await vm.handleSearch()
    await flushPromises()

    expect(searchTmdb).toHaveBeenCalledWith('Inception', 'movie', { silent: true })
    expect(vm.results).toEqual([
      expect.objectContaining({
        id: 27205,
        title: 'Inception',
      }),
    ])
  })

  it('创建订阅命中自动通过时提示等待入库', async () => {
    vi.mocked(createSubscription).mockResolvedValue({
      success: true,
      subscriptionId: 'sub_auto_1',
      status: 'APPROVED',
      autoApproved: true,
    } as never)

    const wrapper = mountView()
    const vm = wrapper.vm as unknown as {
      searchType: 'MOVIE' | 'TV'
      selectedItem: Record<string, any> | null
      showConfirmDialog: boolean
      subscriptionForm: { season: number | null; note: string }
      confirmSubscription: () => Promise<void>
    }

    vm.searchType = 'MOVIE'
    vm.selectedItem = {
      id: 27205,
      title: 'Inception',
      posterPath: '/poster.jpg',
      overview: 'dreams',
    }
    vm.showConfirmDialog = true
    vm.subscriptionForm = { season: null, note: '' }

    await vm.confirmSubscription()
    await flushPromises()

    expect(createSubscription).toHaveBeenCalledWith({
      type: 'MOVIE',
      name: 'Inception',
      tmdbId: '27205',
      season: 0,
      posterPath: '/poster.jpg',
      note: '',
      confirmExisting: false,
    })
    expect(ElMessage.success).toHaveBeenCalledWith('订阅已自动通过，等待入库')
    expect(pushMock).toHaveBeenCalledWith('/console/subscriptions')
  })
})
