/// <reference types="node" />
// 本文件运行在 Node（vitest）侧，需要 Node 全局类型；应用 tsconfig 的 types 仅含 vite/client。
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import net from 'node:net'
import path from 'node:path'

export const GO_INTEGRATION_STARTUP_TIMEOUT_MS = 60000

export interface RunningGoServer {
  baseUrl: string
  dispose: () => Promise<void>
}

async function findFreePort() {
  return await new Promise<number>((resolve, reject) => {
    const server = net.createServer()
    server.unref()
    server.on('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      if (!address || typeof address === 'string') {
        reject(new Error('failed to resolve free port'))
        return
      }
      const { port } = address
      server.close((error) => {
        if (error) {
          reject(error)
          return
        }
        resolve(port)
      })
    })
  })
}

async function waitForHealth(baseUrl: string, child: ChildProcessWithoutNullStreams) {
  const startedAt = Date.now()
  let stdout = ''
  let stderr = ''

  child.stdout.on('data', (chunk) => {
    stdout += chunk.toString()
  })

  child.stderr.on('data', (chunk) => {
    stderr += chunk.toString()
  })

  while (Date.now() - startedAt < GO_INTEGRATION_STARTUP_TIMEOUT_MS) {
    if (child.exitCode !== null) {
      throw new Error(`go integration server exited early:\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    }

    try {
      const response = await fetch(`${baseUrl}/health`)
      if (response.ok) {
        return
      }
    } catch {
      // retry
    }

    await new Promise((resolve) => setTimeout(resolve, 500))
  }

  throw new Error(`go integration server health check timed out:\nstdout:\n${stdout}\nstderr:\n${stderr}`)
}

/**
 * 构造 Web 集成测试的 Go 服务环境，并保留调用方提供的 Go 构建缓存配置。
 */
export function buildGoIntegrationServerEnv(
  databaseUrl: string,
  port: number,
  sourceEnv: NodeJS.ProcessEnv = process.env,
): NodeJS.ProcessEnv {
  return {
    ...sourceEnv,
    DATABASE_URL: databaseUrl,
    PORT: String(port),
    GIN_MODE: 'test',
    JWT_SECRET: 'jjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjj',
    CONFIG_ENCRYPTION_KEY: 'kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk',
    INTERNAL_API_SECRET: '0123456789abcdef0123456789abcdef',
  }
}

export async function startGoIntegrationServer(): Promise<RunningGoServer> {
  const databaseUrl = process.env.EMBER_INTEGRATION_DATABASE_URL
  if (!databaseUrl) {
    throw new Error('EMBER_INTEGRATION_DATABASE_URL is required for web integration tests')
  }

  const port = await findFreePort()
  const baseUrl = `http://127.0.0.1:${port}`
  const apiDir = path.resolve(__dirname, '../../../api')

  const child = spawn('go', ['run', './cmd/webintegrationserver'], {
    cwd: apiDir,
    env: buildGoIntegrationServerEnv(databaseUrl, port),
    stdio: 'pipe',
  })

  await waitForHealth(baseUrl, child)

  return {
    baseUrl,
    dispose: async () => {
      if (child.exitCode !== null) {
        return
      }
      child.kill('SIGTERM')
      await new Promise<void>((resolve) => {
        child.once('exit', () => resolve())
        setTimeout(() => {
          if (child.exitCode === null) {
            child.kill('SIGKILL')
          }
          resolve()
        }, 5000)
      })
    },
  }
}
