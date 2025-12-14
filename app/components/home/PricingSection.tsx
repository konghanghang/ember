export default function PricingSection() {
  return (
    <section
      id="pricing"
      className="py-20 px-4 bg-gradient-to-br from-indigo-50 to-purple-50 dark:from-gray-800 dark:to-gray-900"
    >
      <div className="max-w-7xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            简单透明的定价
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400">
            无隐藏费用，按月付费，随时可停
          </p>
        </div>

        {/* 定价卡片 */}
        <div className="max-w-lg mx-auto">
          <div className="bg-white dark:bg-gray-800 rounded-2xl shadow-xl overflow-hidden border-2 border-indigo-600">
            {/* 推荐标签 */}
            <div className="bg-indigo-600 text-white text-center py-2 text-sm font-semibold">
              当前套餐
            </div>

            <div className="p-8">
              {/* 套餐名称和价格 */}
              <div className="text-center mb-8">
                <h3 className="text-2xl font-bold text-gray-900 dark:text-white mb-4">
                  💎 标准版
                </h3>
                <div className="flex items-baseline justify-center gap-2">
                  <span className="text-5xl font-bold text-indigo-600">10</span>
                  <span className="text-xl text-gray-600 dark:text-gray-400">元/月</span>
                </div>
              </div>

              {/* 功能列表 */}
              <ul className="space-y-4 mb-8">
                {[
                  '海量 4K 高清影视资源',
                  '热门剧集同步更新',
                  '基于 CloudFlare CDN 加速',
                  '支持 5 个设备同时观看',
                  'Emby 全平台客户端支持',
                  '7x24 小时在线服务',
                ].map((feature, index) => (
                  <li key={index} className="flex items-start">
                    <svg
                      className="w-6 h-6 text-green-500 mr-3 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M5 13l4 4L19 7"
                      />
                    </svg>
                    <span className="text-gray-700 dark:text-gray-300">{feature}</span>
                  </li>
                ))}
              </ul>

              {/* CTA 按钮 */}
              <a
                href="https://t.me/NextNewEP_emby_chat"
                target="_blank"
                rel="noopener noreferrer"
                className="block w-full px-8 py-4 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors font-medium text-lg text-center shadow-lg hover:shadow-xl"
              >
                📱 加入群组立即购买
              </a>
            </div>
          </div>

          {/* 提示信息 */}
          <p className="text-center mt-6 text-sm text-gray-500 dark:text-gray-400">
            💡 适合有网络加速工具的用户 · 后续将推出优化线路版本
          </p>
        </div>
      </div>
    </section>
  )
}
