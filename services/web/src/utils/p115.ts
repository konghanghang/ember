const appTypeBySSOEnt: Readonly<Record<string, string>> = {
  A1: 'web',
  D1: 'ios',
  D2: 'bios',
  D3: '115ios',
  F1: 'android',
  F2: 'bandroid',
  F3: '115android',
  H1: 'ipad',
  H2: 'bipad',
  H3: '115ipad',
  I1: 'tv',
  I2: 'apple_tv',
  M1: 'qandroid',
  N1: 'qios',
  O1: 'qipad',
  P1: 'os_windows',
  P2: 'os_mac',
  P3: 'os_linux',
  R1: 'wechatmini',
  R2: 'alipaymini',
  S1: 'harmony',
}

/** 从 Cookie UID 的 ssoent 本地识别客户端类型，未知编码不做猜测。 */
export function detectP115CookieAppType(cookieHeader: string): string | null {
  const uidValues = cookieHeader
    .split(';')
    .map(part => part.trim())
    .filter(Boolean)
    .flatMap((part) => {
      const separator = part.indexOf('=')
      if (separator < 0 || part.slice(0, separator).trim() !== 'UID') return []
      return [part.slice(separator + 1).trim()]
    })

  if (uidValues.length !== 1 || !uidValues[0]) return null
  const ssoent = uidValues[0].split('_')[1]?.trim().toUpperCase()
  if (!ssoent) return null
  return appTypeBySSOEnt[ssoent] ?? null
}
