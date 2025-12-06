import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = {
  title: 'Ember - Emby 用户管理系统',
  description: 'Emby 邀请码注册和用户到期管理系统',
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="zh-CN">
      <body className="antialiased">{children}</body>
    </html>
  )
}
