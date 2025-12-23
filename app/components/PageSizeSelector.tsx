interface PageSizeSelectorProps {
  pageSize: number
  onPageSizeChange: (size: number) => void
  options?: number[]
  className?: string
}

export default function PageSizeSelector({
  pageSize,
  onPageSizeChange,
  options = [10, 20, 50, 100],
  className = '',
}: PageSizeSelectorProps) {
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <span className="text-sm text-gray-600 dark:text-gray-400">每页显示</span>
      <select
        value={pageSize}
        onChange={(e) => onPageSizeChange(Number(e.target.value))}
        className="px-3 py-2 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 focus:ring-2 focus:ring-emerald-500 focus:border-transparent transition-colors"
      >
        {options.map((size) => (
          <option key={size} value={size}>
            {size} 条
          </option>
        ))}
      </select>
    </div>
  )
}
