import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import net from 'node:net'
import os from 'node:os'
import path from 'node:path'

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

  while (Date.now() - startedAt < 30000) {
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

export async function startGoIntegrationServer(): Promise<RunningGoServer> {
  const databaseUrl = process.env.EMBER_INTEGRATION_DATABASE_URL
  if (!databaseUrl) {
    throw new Error('EMBER_INTEGRATION_DATABASE_URL is required for web integration tests')
  }

  const port = await findFreePort()
  const baseUrl = `http://127.0.0.1:${port}`
  const apiDir = path.resolve(__dirname, '../../../api')
  const goCacheDir = path.join(os.tmpdir(), 'ember-go-cache-web-integration')

  const child = spawn('go', ['run', './cmd/webintegrationserver'], {
    cwd: apiDir,
    env: {
      ...process.env,
      DATABASE_URL: databaseUrl,
      PORT: String(port),
      GIN_MODE: 'test',
      JWT_SECRET: 'jjjjjjjjjjjjjjjjjjjjjjjjjjjjjjjj',
      INTERNAL_API_SECRET: '0123456789abcdef0123456789abcdef',
      GOCACHE: goCacheDir,
    },
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
