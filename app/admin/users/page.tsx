'use client'

import { useEffect, useState } from 'react'
import { getUsers, extendExpiry, toggleUserStatus, deleteUser } from '@/app/actions/users'
import { formatDateTime } from '@/lib/utils'
import Pagination from '@/app/components/Pagination'
import PageSizeSelector from '@/app/components/PageSizeSelector'

interface User {
  id: string
  username: string
  email: string
  embyId: string
  isActive: boolean
  expiresAt: Date | null
  createdAt: Date
  invite?: {
    code: string
  }
}

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [searchTerm, setSearchTerm] = useState('')

  // 分页状态
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [total, setTotal] = useState(0)

  // 延长到期时间对话框状态
  const [extendDialogOpen, setExtendDialogOpen] = useState(false)
  const [selectedUser, setSelectedUser] = useState<User | null>(null)
  const [extendDays, setExtendDays] = useState(30)
  const [extending, setExtending] = useState(false)

  useEffect(() => {
    loadUsers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentPage, pageSize])

  async function loadUsers() {
    setLoading(true)
    const result = await getUsers({
      search: searchTerm || undefined,
      page: currentPage,
      pageSize,
    })
    if (result.success && result.users) {
      setUsers(result.users as User[])
      setTotal(result.total || 0)
    }
    setLoading(false)
  }

  async function handleSearch() {
    setCurrentPage(1) // 搜索时重置到第一页
    await loadUsers()
  }

  // 延长到期时间
  async function handleExtendExpiry() {
    if (!selectedUser) return

    setExtending(true)
    const result = await extendExpiry(selectedUser.id, extendDays)

    if (result.success) {
      await loadUsers()
      setExtendDialogOpen(false)
      setSelectedUser(null)
      setExtendDays(30)
    } else {
      alert(result.error || '延长失败')
    }

    setExtending(false)
  }

  // 切换用户状态
  async function handleToggleStatus(user: User) {
    const action = user.isActive ? '禁用' : '启用'
    if (!confirm(`确定${action}用户 ${user.username}？`)) return

    const result = await toggleUserStatus(user.id)

    if (result.success) {
      await loadUsers()
    } else {
      alert(result.error || '操作失败')
    }
  }

  // 删除用户
  async function handleDelete(user: User) {
    if (!confirm(`确定删除用户 ${user.username}？\n\n此操作将同时删除 Emby 账号，且不可恢复！`)) return

    const result = await deleteUser(user.id)

    if (result.success) {
      await loadUsers()
    } else {
      alert(result.error || '删除失败')
    }
  }

  // 判断用户是否已过期
  function isExpired(expiresAt: Date | null): boolean {
    if (!expiresAt) return false
    return new Date(expiresAt) < new Date()
  }

  if (loading) {
    return <div className="text-center py-12">加载中...</div>
  }

  return (
    <div>
      {/* 页面标题和搜索 */}
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-6 gap-4">
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white">
          用户管理
        </h1>

        {/* 搜索框 */}
        <div className="flex gap-2 w-full md:w-auto">
          <input
            type="text"
            placeholder="搜索用户名..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            className="px-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white flex-1 md:w-64"
          />
          <button
            onClick={handleSearch}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white font-medium rounded-lg transition-colors"
          >
            搜索
          </button>
        </div>
      </div>

      {/* 用户列表 */}
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-md overflow-hidden">
        {users.length === 0 ? (
          <div className="text-center py-12 text-gray-500 dark:text-gray-400">
            {searchTerm ? '未找到匹配的用户' : '暂无用户'}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 dark:bg-gray-700">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    用户名
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    邮箱
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    Emby ID
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    状态
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    到期时间
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    注册时间
                  </th>
                  <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                    操作
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {users.map((user) => (
                  <tr key={user.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm font-medium text-gray-900 dark:text-gray-100">
                        {user.username}
                      </div>
                      {user.invite && (
                        <div className="text-xs text-gray-500 dark:text-gray-400">
                          邀请码: {user.invite.code}
                        </div>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 dark:text-gray-100">
                      {user.email}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span className="font-mono text-xs text-gray-600 dark:text-gray-400">
                        {user.embyId}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span
                        className={`px-2 py-1 text-xs rounded ${
                          user.isActive
                            ? 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200'
                            : 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200'
                        }`}
                      >
                        {user.isActive ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <div className="text-sm text-gray-900 dark:text-gray-100">
                        {user.expiresAt ? formatDateTime(user.expiresAt) : '永久'}
                      </div>
                      {user.expiresAt && isExpired(user.expiresAt) && (
                        <span className="text-xs text-red-600 dark:text-red-400">
                          已过期
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 dark:text-gray-400">
                      {formatDateTime(user.createdAt)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-2">
                      {/* 延长到期 */}
                      <button
                        onClick={() => {
                          setSelectedUser(user)
                          setExtendDialogOpen(true)
                        }}
                        className="text-indigo-600 hover:text-indigo-900 dark:text-indigo-400 dark:hover:text-indigo-300"
                      >
                        延长
                      </button>

                      {/* 启用/禁用 */}
                      <button
                        onClick={() => handleToggleStatus(user)}
                        className="text-yellow-600 hover:text-yellow-900 dark:text-yellow-400 dark:hover:text-yellow-300"
                      >
                        {user.isActive ? '禁用' : '启用'}
                      </button>

                      {/* 删除 */}
                      <button
                        onClick={() => handleDelete(user)}
                        className="text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 分页控件 */}
      {total > 0 && (
        <div className="mt-6 flex flex-col sm:flex-row items-center justify-between gap-4">
          <PageSizeSelector
            pageSize={pageSize}
            onPageSizeChange={(size) => {
              setPageSize(size)
              setCurrentPage(1)
            }}
          />
          <Pagination
            currentPage={currentPage}
            totalPages={Math.ceil(total / pageSize)}
            onPageChange={setCurrentPage}
          />
          <div className="text-sm text-gray-600 dark:text-gray-400">
            共 {total} 条记录
          </div>
        </div>
      )}

      {/* 延长到期时间对话框 */}
      {extendDialogOpen && selectedUser && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 w-full max-w-md">
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-4">
              延长到期时间
            </h2>

            <div className="mb-4">
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
                用户: <span className="font-medium text-gray-900 dark:text-white">{selectedUser.username}</span>
              </p>
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
                当前到期时间: <span className="font-medium text-gray-900 dark:text-white">
                  {selectedUser.expiresAt ? formatDateTime(selectedUser.expiresAt) : '永久'}
                </span>
              </p>

              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                延长天数
              </label>
              <input
                type="number"
                min="1"
                value={extendDays}
                onChange={(e) => setExtendDays(parseInt(e.target.value))}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg dark:bg-gray-700 dark:text-white"
              />
            </div>

            <div className="flex justify-end gap-3">
              <button
                onClick={() => {
                  setExtendDialogOpen(false)
                  setSelectedUser(null)
                  setExtendDays(30)
                }}
                className="px-4 py-2 text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleExtendExpiry}
                disabled={extending}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:bg-indigo-400 text-white font-medium rounded-lg transition-colors"
              >
                {extending ? '处理中...' : '确认延长'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
