export default function FeaturesSection() {
  const features = [
    {
      icon: '👥',
      title: '用户管理',
      description: '统一管理 Emby 用户账号，支持批量操作、权限控制和状态监控',
    },
    {
      icon: '📋',
      title: '订阅系统',
      description: '灵活的订阅计划管理，自动化到期提醒和续费流程，提升运营效率',
    },
    {
      icon: '🎫',
      title: '邀请码管理',
      description: '精细化控制新用户注册，支持批量生成、使用追踪和权限配置',
    },
  ]

  return (
    <section id="features" className="py-20 px-4 bg-white dark:bg-gray-900">
      <div className="max-w-7xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            核心功能
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
            为 Emby 服务器管理员量身打造的运营工具
          </p>
        </div>

        {/* 功能卡片 */}
        <div className="grid md:grid-cols-3 gap-8">
          {features.map((feature, index) => (
            <div
              key={index}
              className="bg-gray-50 dark:bg-gray-800 rounded-2xl p-8 hover:shadow-lg transition-shadow border border-gray-200 dark:border-gray-700"
            >
              {/* 图标 */}
              <div className="text-5xl mb-4">{feature.icon}</div>

              {/* 标题 */}
              <h3 className="text-xl font-bold text-gray-900 dark:text-white mb-3">
                {feature.title}
              </h3>

              {/* 描述 */}
              <p className="text-gray-600 dark:text-gray-400 leading-relaxed">
                {feature.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
