export default function Home() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-gray-800 px-4">
      <div className="text-center max-w-2xl">
        {/* Logo 和标题 */}
        <h1 className="text-6xl font-bold text-gray-900 dark:text-white mb-4">
          Ember
        </h1>

        {/* 产品介绍 */}
        <p className="text-xl text-gray-600 dark:text-gray-300 mb-2">
          Emby 媒体服务器用户管理系统
        </p>
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-8">
          统一管理 Emby 用户账号、订阅和邀请码
        </p>

        {/* 按钮组 - 三个平等的入口 */}
        <div className="flex flex-col sm:flex-row gap-4 justify-center">
          <a
            href="/user/login"
            className="px-6 py-3 bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 transition-colors font-medium"
          >
            用户登录
          </a>
          <a
            href="/register"
            className="px-6 py-3 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors font-medium"
          >
            新用户注册
          </a>
          <a
            href="/login"
            className="px-6 py-3 bg-gray-200 text-gray-800 dark:bg-gray-700 dark:text-gray-200 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors font-medium"
          >
            管理员入口
          </a>
        </div>
      </div>
    </div>
  )
}
