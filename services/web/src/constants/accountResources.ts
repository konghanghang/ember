export type AccountResourceIcon = 'notify' | 'group' | 'wiki'

export interface AccountResourceLink {
  key: string
  title: string
  description: string
  href: string
  icon: AccountResourceIcon
}

export const accountResourceLinks: AccountResourceLink[] = [
  {
    key: 'notify-channel',
    title: '通知频道',
    description: '获取最新入库通知与系统动态',
    href: 'https://t.me/NextNewEP',
    icon: 'notify'
  },
  {
    key: 'community-group',
    title: '交流群组',
    description: '加入社区讨论、反馈问题和求助',
    href: 'https://t.me/NextNewEP_emby_chat',
    icon: 'group'
  },
  {
    key: 'wiki',
    title: '使用 Wiki',
    description: '查看常见问题、设备配置和使用说明',
    href: 'https://github.com/konghang/ember/wiki',
    icon: 'wiki'
  }
]
