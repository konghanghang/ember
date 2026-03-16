import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Ember Docs',
  description: 'Ember 面向用户和部署者的公开文档',
  cleanUrls: true,
  lang: 'zh-CN',
  lastUpdated: true,
  themeConfig: {
    nav: [
      { text: '快速开始', link: '/getting-started' },
      { text: '部署', link: '/deployment' },
      { text: '配置', link: '/configuration' },
      { text: '功能', link: '/features/overview' },
      { text: '集成', link: '/integrations/telegram' },
      { text: '管理后台', link: '/admin/overview' },
      { text: 'FAQ', link: '/faq' }
    ],
    sidebar: [
      {
        text: '开始',
        items: [
          { text: '首页', link: '/' },
          { text: '快速开始', link: '/getting-started' },
          { text: '部署说明', link: '/deployment' },
          { text: '配置说明', link: '/configuration' },
          { text: '常见问题', link: '/faq' },
          { text: '更新记录', link: '/changelog' }
        ]
      },
      {
        text: '功能',
        items: [
          { text: '功能总览', link: '/features/overview' },
          { text: '用户控制台', link: '/features/user-console' },
          { text: 'Telegram Bot', link: '/features/telegram-bot' },
          { text: '支付与续费', link: '/features/payments' },
          { text: '追剧日历', link: '/features/tv-calendar' }
        ]
      },
      {
        text: '集成',
        items: [
          { text: 'Telegram', link: '/integrations/telegram' },
          { text: 'Emby', link: '/integrations/emby' },
          { text: 'Stripe', link: '/integrations/stripe' },
          { text: 'MoviePilot', link: '/integrations/moviepilot' }
        ]
      },
      {
        text: '管理后台',
        items: [
          { text: '总览', link: '/admin/overview' },
          { text: '用户管理', link: '/admin/users' },
          { text: '兑换码管理', link: '/admin/redemption-codes' },
          { text: '设置中心', link: '/admin/settings' },
          { text: '会话与设备', link: '/admin/sessions-and-devices' }
        ]
      }
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/konghanghang/ember' }
    ],
    footer: {
      message: '公开文档仅覆盖用户与部署者视角',
      copyright: 'Ember'
    }
  }
})
