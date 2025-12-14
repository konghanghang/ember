import Link from 'next/link'

export default function Footer() {
  const currentYear = new Date().getFullYear()

  return (
    <footer className="bg-gray-900 text-gray-400 py-12 px-4">
      <div className="max-w-7xl mx-auto">
        <div className="grid md:grid-cols-4 gap-8 mb-8">
          {/* Logo 和简介 */}
          <div className="md:col-span-2">
            <h3 className="text-2xl font-bold text-white mb-3">Ember</h3>
            <p className="text-sm leading-relaxed mb-4">
              专注于提供优质流媒体影音体验的用户管理平台。海量 4K 资源，热门剧集同步更新，邀请制精品社区。
            </p>
            <a
              href="https://t.me/NextNewEP_emby_chat"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors text-sm font-medium"
            >
              📱 Telegram 群组
            </a>
          </div>

          {/* 快捷链接 */}
          <div>
            <h4 className="text-white font-semibold mb-3">快捷链接</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <a href="#features" className="hover:text-white transition-colors">
                  核心特色
                </a>
              </li>
              <li>
                <a href="#pricing" className="hover:text-white transition-colors">
                  套餐定价
                </a>
              </li>
              <li>
                <a href="#quickstart" className="hover:text-white transition-colors">
                  快速开始
                </a>
              </li>
              <li>
                <a href="#faq" className="hover:text-white transition-colors">
                  常见问题
                </a>
              </li>
            </ul>
          </div>

          {/* 登录入口 */}
          <div>
            <h4 className="text-white font-semibold mb-3">用户入口</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/user/login" className="hover:text-white transition-colors">
                  用户登录
                </Link>
              </li>
              <li>
                <Link href="/register" className="hover:text-white transition-colors">
                  新用户注册
                </Link>
              </li>
              <li>
                <Link href="/login" className="hover:text-white transition-colors">
                  管理员登录
                </Link>
              </li>
            </ul>
          </div>
        </div>

        {/* 底部版权 */}
        <div className="border-t border-gray-800 pt-8 text-center text-sm">
          <p>© {currentYear} Ember. All rights reserved.</p>
        </div>
      </div>
    </footer>
  )
}
