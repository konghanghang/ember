import Link from 'next/link'

export default function Footer() {
  const currentYear = new Date().getFullYear()

  return (
    <footer className="bg-gray-900 text-gray-400 py-12 px-4">
      <div className="max-w-7xl mx-auto">
        <div className="grid md:grid-cols-3 gap-8 mb-8">
          {/* Logo 和简介 */}
          <div>
            <h3 className="text-2xl font-bold text-white mb-3">Ember</h3>
            <p className="text-sm leading-relaxed">
              专为 Emby 媒体服务器设计的用户管理系统，简化运营工作，提升管理效率。
            </p>
          </div>

          {/* 快捷链接 */}
          <div>
            <h4 className="text-white font-semibold mb-3">快捷链接</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <a href="#features" className="hover:text-white transition-colors">
                  功能介绍
                </a>
              </li>
              <li>
                <a href="#quickstart" className="hover:text-white transition-colors">
                  快速开始
                </a>
              </li>
              <li>
                <Link href="/register" className="hover:text-white transition-colors">
                  用户注册
                </Link>
              </li>
            </ul>
          </div>

          {/* 登录入口 */}
          <div>
            <h4 className="text-white font-semibold mb-3">登录入口</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/user/login" className="hover:text-white transition-colors">
                  用户登录
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
