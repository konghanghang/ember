import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
  output: 'standalone', // Docker 部署优化
  experimental: {
    serverActions: {
      bodySizeLimit: '2mb',
    },
  },
}

export default nextConfig
