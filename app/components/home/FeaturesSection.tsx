export default function FeaturesSection() {
  const features = [
    {
      icon: '🎬',
      title: '海量影视资源',
      description: '涵盖电影、剧集、纪录片、动漫等多种类型，支持 4K 超高清播放',
    },
    {
      icon: '⚡',
      title: '新番同步追更',
      description: '热门剧集及时更新，不错过每一集精彩内容，第一时间观看最新剧集',
    },
    {
      icon: '🎫',
      title: '邀请制社区',
      description: '基于邀请码的精品社区，高质量用户群体，共享优质观影体验',
    },
  ]

  return (
    <section id="features" className="py-20 px-4 bg-white dark:bg-gray-900">
      <div className="max-w-7xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            核心特色
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400 max-w-2xl mx-auto">
            优质流媒体资源，极致观影体验
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
