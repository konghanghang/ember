import { PrismaClient } from '@prisma/client'
import { PrismaPg } from '@prisma/adapter-pg'
import { Pool } from 'pg'
import bcrypt from 'bcryptjs'

// Prisma 7 需要使用 adapter
const pool = new Pool({ connectionString: process.env.DATABASE_URL })
const adapter = new PrismaPg(pool)
const prisma = new PrismaClient({ adapter })

async function main() {
  console.log('🌱 开始初始化数据库...')

  // 创建默认管理员账号
  // 优先使用环境变量配置的密码，否则使用默认密码
  const plainPassword = process.env.ADMIN_DEFAULT_PASSWORD || 'admin123'
  const password = await bcrypt.hash(plainPassword, 10)

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
  console.log(`   密码: ${plainPassword}`)
  console.log(`   ID: ${admin.id}`)
  console.log('')
  if (plainPassword === 'admin123') {
    console.log('⚠️  警告：当前使用默认密码 "admin123"')
    console.log('   生产环境请在 .env 中设置 ADMIN_DEFAULT_PASSWORD')
    console.log('')
  }
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
