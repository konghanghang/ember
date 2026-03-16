import { defineConfig } from 'vitepress'

const base = process.env.VITEPRESS_BASE || '/'

export default defineConfig({
  title: 'Ember Docs',
  description: 'Ember 面向普通用户的网站使用手册',
  base,
  cleanUrls: true,
  lang: 'zh-CN',
  lastUpdated: true,
  themeConfig: {
    nav: [
      { text: '播放器推荐', link: '/features/players' },
      { text: '第一次使用', link: '/getting-started' },
      { text: '用户使用', link: '/features/overview' },
      { text: 'Telegram Bot', link: '/features/telegram-bot' },
      { text: 'FAQ', link: '/faq' }
    ],
    sidebar: [
      {
        text: '开始',
        items: [
          { text: '首页', link: '/' },
          { text: '第一次使用', link: '/getting-started' },
        ]
      },
      {
        text: '播放器推荐',
        items: [
          { text: '播放器推荐', link: '/features/players' },
        ]
      },
      {
        text: '用户使用',
        items: [
          { text: '功能地图', link: '/features/overview' },
          { text: '控制台总览', link: '/features/user-console' },
          { text: '我的账号', link: '/features/my-account' },
          { text: '订阅管理', link: '/features/subscriptions' },
          { text: '播放排行榜', link: '/features/rankings' },
          { text: '媒体库', link: '/features/library' },
          { text: '追剧日历', link: '/features/tv-calendar' },
          { text: '续费中心', link: '/features/payments' },
          { text: 'Telegram Bot', link: '/features/telegram-bot' },
        ]
      },
      {
        text: '帮助',
        items: [
          { text: '社群与通知', link: '/community' },
          { text: '常见问题', link: '/faq' },
          { text: '更新记录', link: '/changelog' }
        ]
      }
    ],
    footer: {
      message: '公开文档只覆盖普通用户使用视角',
      copyright: 'Ember'
    }
  }
})
