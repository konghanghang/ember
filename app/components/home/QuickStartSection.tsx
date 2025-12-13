import Link from 'next/link'

export default function QuickStartSection() {
  const steps = [
    {
      number: '1',
      title: '注册账号',
      description: '使用邀请码注册 Ember 账号，管理员审核通过后即可使用',
    },
    {
      number: '2',
      title: '选择订阅',
      description: '根据需求选择合适的订阅计划，系统自动创建 Emby 账号',
    },
    {
      number: '3',
      title: '开始使用',
      description: '使用分配的账号登录 Emby，享受海量影视资源',
    },
  ]

  return (
    <section
      id="quickstart"
      className="py-20 px-4 bg-gradient-to-br from-indigo-50 to-purple-50 dark:from-gray-800 dark:to-gray-900"
    >
      <div className="max-w-7xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            快速开始
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
            三步即可开始使用 Emby 媒体服务
          </p>
        </div>

        {/* 步骤 */}
        <div className="grid md:grid-cols-3 gap-8 mb-12">
          {steps.map((step, index) => (
            <div key={index} className="relative">
              {/* 连接线（桌面端） */}
              {index < steps.length - 1 && (
                <div className="hidden md:block absolute top-12 left-1/2 w-full h-0.5 bg-indigo-200 dark:bg-indigo-800 -z-10" />
              )}

              <div className="bg-white dark:bg-gray-800 rounded-2xl p-8 text-center relative">
                {/* 步骤编号 */}
                <div className="inline-flex items-center justify-center w-16 h-16 bg-indigo-600 text-white rounded-full text-2xl font-bold mb-4">
                  {step.number}
                </div>

                {/* 标题 */}
                <h3 className="text-xl font-bold text-gray-900 dark:text-white mb-3">
                  {step.title}
                </h3>

                {/* 描述 */}
                <p className="text-gray-600 dark:text-gray-400 leading-relaxed">
                  {step.description}
                </p>
              </div>
            </div>
          ))}
        </div>

        {/* CTA */}
        <div className="text-center">
          <Link
            href="/register"
            className="inline-block px-8 py-4 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors font-medium text-lg shadow-lg hover:shadow-xl"
          >
            立即注册
          </Link>
        </div>
      </div>
    </section>
  )
}
