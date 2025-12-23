import Link from 'next/link'

export default function NotFound() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900 dark:to-gray-800 px-4">
      <div className="text-center">
        <h1 className="text-9xl font-bold text-gray-300 dark:text-gray-700">
          404
        </h1>
        <h2 className="text-3xl font-bold text-gray-900 dark:text-white mt-4">
          页面未找到
        </h2>
        <p className="text-gray-600 dark:text-gray-400 mt-4 mb-8">
          抱歉，您访问的页面不存在
        </p>
        <Link
          href="/"
          className="inline-flex items-center px-6 py-3 bg-emerald-600 hover:bg-emerald-700 text-white font-medium rounded-lg transition-colors"
        >
          返回首页
        </Link>
      </div>
    </div>
  )
}
