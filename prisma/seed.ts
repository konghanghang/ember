import { PrismaClient } from '@prisma/client'
import bcrypt from 'bcryptjs'

const prisma = new PrismaClient()

async function main() {
  console.log('🌱 开始初始化数据库...')

  // 创建默认管理员账号
  const password = await bcrypt.hash('admin123', 10)

  const admin = await prisma.admin.upsert({
    where: { username: 'admin' },
    update: {},
    create: {
      username: 'admin',
      password,
    },
  })

  console.log('✅ 创建默认管理员账号：')
  console.log(`   用户名: ${admin.username}`)
  console.log(`   密码: admin123`)
  console.log(`   ID: ${admin.id}`)
  console.log('')
  console.log('🎉 数据库初始化完成！')
}

main()
  .then(async () => {
    await prisma.$disconnect()
  })
  .catch(async (e) => {
    console.error('❌ 数据库初始化失败：', e)
    await prisma.$disconnect()
    process.exit(1)
  })
