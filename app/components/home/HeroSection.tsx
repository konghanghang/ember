import Link from 'next/link'

export default function HeroSection() {
  return (
    <section className="relative pt-32 pb-20 px-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800">
      <div className="max-w-7xl mx-auto text-center">
        {/* 主标题 */}
        <h1 className="text-5xl md:text-6xl lg:text-7xl font-bold text-gray-900 dark:text-white mb-6">
          Ember
        </h1>

        {/* 副标题 */}
        <p className="text-2xl md:text-3xl text-gray-700 dark:text-gray-200 mb-4">
          统一管理你的 Emby 媒体服务器
        </p>

        <p className="text-lg text-gray-600 dark:text-gray-400 mb-12 max-w-2xl mx-auto">
          简化用户账号管理、订阅计划、邀请码等运营工作，让你专注于为用户提供优质的媒体服务
        </p>

        {/* CTA 按钮组 */}
        <div className="flex flex-col sm:flex-row gap-4 justify-center items-center">
          <Link
            href="/register"
            className="px-8 py-4 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors font-medium text-lg shadow-lg hover:shadow-xl"
          >
            开始使用
          </Link>
          <Link
            href="/user/login"
            className="px-8 py-4 bg-white dark:bg-gray-800 text-gray-900 dark:text-white rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors font-medium text-lg border border-gray-200 dark:border-gray-700"
          >
            用户登录
          </Link>
        </div>

        {/* 提示文字 */}
        <p className="mt-8 text-sm text-gray-500 dark:text-gray-400">
          已有账号？
          <Link
            href="/user/login"
            className="text-indigo-600 dark:text-indigo-400 hover:underline ml-1"
          >
            立即登录
          </Link>
        </p>
      </div>
    </section>
  )
}
