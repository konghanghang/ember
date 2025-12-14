'use client'

import { useState } from 'react'

export default function FAQSection() {
  const [openIndex, setOpenIndex] = useState<number | null>(0)

  const faqs = [
    {
      question: '需要什么条件才能流畅观看？',
      answer:
        '建议使用稳定的网络加速工具（如 VPN、机场等）。服务基于 CloudFlare CDN，对全球网络友好，但国内直连可能需要加速。',
    },
    {
      question: '支持哪些设备？',
      answer:
        '支持 Emby 全平台客户端，包括 iOS、Android、Apple TV、Android TV、Web 浏览器、Windows、macOS 等。可同时在 5 个设备上观看。',
    },
    {
      question: '如何购买和注册？',
      answer:
        '加入我们的 Telegram 群组，群内联系管理员购买邀请码（10元/月）。购买后使用邀请码注册账号，即可立即使用。',
    },
    {
      question: '资源更新频率如何？',
      answer:
        '热门剧集同步更新，Netflix、Disney+ 等平台的新剧通常在播出后数小时内即可观看。电影和纪录片也会持续更新。',
    },
    {
      question: '后续会推出优化线路吗？',
      answer:
        '是的！我们计划推出优化线路版本（18元/月），国内用户无需加速工具即可流畅观看。请关注群组公告了解最新动态。',
    },
  ]

  return (
    <section id="faq" className="py-20 px-4 bg-white dark:bg-gray-900">
      <div className="max-w-3xl mx-auto">
        {/* 标题 */}
        <div className="text-center mb-16">
          <h2 className="text-3xl md:text-4xl font-bold text-gray-900 dark:text-white mb-4">
            常见问题
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-400">
            快速了解我们的服务
          </p>
        </div>

        {/* FAQ 列表 */}
        <div className="space-y-4">
          {faqs.map((faq, index) => (
            <div
              key={index}
              className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden"
            >
              {/* 问题 */}
              <button
                onClick={() => setOpenIndex(openIndex === index ? null : index)}
                className="w-full px-6 py-4 text-left bg-gray-50 dark:bg-gray-800 hover:bg-gray-100 dark:hover:bg-gray-750 transition-colors flex justify-between items-center"
              >
                <span className="font-semibold text-gray-900 dark:text-white">
                  {faq.question}
                </span>
                <svg
                  className={`w-5 h-5 text-gray-500 transition-transform ${
                    openIndex === index ? 'transform rotate-180' : ''
                  }`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 9l-7 7-7-7"
                  />
                </svg>
              </button>

              {/* 答案 */}
              {openIndex === index && (
                <div className="px-6 py-4 bg-white dark:bg-gray-900">
                  <p className="text-gray-600 dark:text-gray-400 leading-relaxed">
                    {faq.answer}
                  </p>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
