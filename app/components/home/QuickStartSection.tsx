export default function QuickStartSection() {
  const steps = [
    {
      number: '1',
      title: '加入 Telegram 群组',
      description: '点击下方链接加入我们的社区，与管理员沟通您的需求',
    },
    {
      number: '2',
      title: '购买邀请码',
      description: '群内联系管理员购买邀请码（10元/月），账号权限已自动配置',
    },
    {
      number: '3',
      title: '开始观影',
      description: '使用邀请码注册账号，下载 Emby 客户端即可享受高清影音',
    },
  ]

  return (
    <section
      id="quickstart"
      className="py-20 px-4 bg-white dark:bg-gray-900"
    >
      <div className="max-w-7xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            快速开始
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
            三步即可开始享受优质流媒体服务
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

              <div className="bg-gray-50 dark:bg-gray-800 rounded-2xl p-8 text-center relative border border-gray-200 dark:border-gray-700">
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
          <a
            href="https://t.me/NextNewEP_emby_chat"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-block px-8 py-4 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors font-medium text-lg shadow-lg hover:shadow-xl"
          >
            📱 立即加入 Telegram 群组
          </a>
        </div>
      </div>
    </section>
  )
}
