import { describe, expect, it } from 'vitest'

import { detectP115CookieAppType } from './p115'

describe('detectP115CookieAppType', () => {
  it.each([
    ['UID=100_A1_1700000000; CID=fake', 'web'],
    ['UID=100_D1_1700000000', 'ios'],
    ['UID=100_D2_1700000000', 'bios'],
    ['UID=100_D3_1700000000', '115ios'],
    ['CID=fake; UID=100_F1_1700000000; SEID=fake', 'android'],
    ['UID=100_F2_1700000000', 'bandroid'],
    ['UID=100_F3_1700000000', '115android'],
    ['UID=100_H1_1700000000', 'ipad'],
    ['UID=100_H2_1700000000', 'bipad'],
    ['UID=100_H3_1700000000', '115ipad'],
    ['UID=100_I1_1700000000', 'tv'],
    ['UID=100_I2_1700000000', 'apple_tv'],
    ['UID=100_M1_1700000000', 'qandroid'],
    ['UID=100_N1_1700000000', 'qios'],
    ['UID=100_O1_1700000000', 'qipad'],
    ['UID=100_P1_1700000000', 'os_windows'],
    ['UID=100_P2_1700000000', 'os_mac'],
    ['UID=100_P3_1700000000', 'os_linux'],
    ['UID=100_R1_1700000000', 'wechatmini'],
    ['UID=100_R2_1700000000', 'alipaymini'],
    ['UID=100_S1_1700000000', 'harmony'],
    ['UID=100_f1_1700000000', 'android'],
  ])('从 UID 的 ssoent 识别客户端类型', (cookie, expected) => {
    expect(detectP115CookieAppType(cookie)).toBe(expected)
  })

  it.each([
    'UID=100_A2_1700000000',
    'UID=100',
    'UID=100__1700000000',
    'UID=100_A1_1700000000; UID=200_F1_1700000000',
    'CID=fake; SEID=fake',
  ])('未知或不完整 Cookie 不伪造客户端类型', (cookie) => {
    expect(detectP115CookieAppType(cookie)).toBeNull()
  })
})
