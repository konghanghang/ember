interface PaginationProps {
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
  className?: string
}

export default function Pagination({
  currentPage,
  totalPages,
  onPageChange,
  className = '',
}: PaginationProps) {
  // 生成页码列表（带省略号）
  const getPageNumbers = () => {
    const pages: (number | string)[] = []
    const showMax = 7 // 最多显示 7 个页码

    if (totalPages <= showMax) {
      // 总页数少，全部显示
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i)
      }
    } else {
      // 总页数多，显示部分+省略号
      if (currentPage <= 3) {
        // 当前页在前面：1 2 3 4 5 ... 10
        for (let i = 1; i <= 5; i++) {
          pages.push(i)
        }
        pages.push('...')
        pages.push(totalPages)
      } else if (currentPage >= totalPages - 2) {
        // 当前页在后面：1 ... 6 7 8 9 10
        pages.push(1)
        pages.push('...')
        for (let i = totalPages - 4; i <= totalPages; i++) {
          pages.push(i)
        }
      } else {
        // 当前页在中间：1 ... 4 5 6 ... 10
        pages.push(1)
        pages.push('...')
        for (let i = currentPage - 1; i <= currentPage + 1; i++) {
          pages.push(i)
        }
        pages.push('...')
        pages.push(totalPages)
      }
    }

    return pages
  }

  const pageNumbers = getPageNumbers()

  if (totalPages <= 1) {
    return null // 只有一页时不显示分页
  }

  return (
    <div className={`flex items-center justify-center gap-2 ${className}`}>
      {/* 上一页 */}
      <button
        onClick={() => onPageChange(currentPage - 1)}
        disabled={currentPage === 1}
        className="px-3 py-2 text-sm font-medium rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        上一页
      </button>

      {/* 页码列表 */}
      <div className="flex items-center gap-1">
        {pageNumbers.map((page, index) => {
          if (page === '...') {
            return (
              <span
                key={`ellipsis-${index}`}
                className="px-3 py-2 text-gray-500 dark:text-gray-400"
              >
                ...
              </span>
            )
          }

          const pageNum = page as number
          const isActive = pageNum === currentPage

          return (
            <button
              key={pageNum}
              onClick={() => onPageChange(pageNum)}
              className={`min-w-[40px] px-3 py-2 text-sm font-medium rounded-lg transition-colors ${
                isActive
                  ? 'bg-emerald-600 text-white'
                  : 'border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700'
              }`}
            >
              {pageNum}
            </button>
          )
        })}
      </div>

      {/* 下一页 */}
      <button
        onClick={() => onPageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
        className="px-3 py-2 text-sm font-medium rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        下一页
      </button>
    </div>
  )
}
