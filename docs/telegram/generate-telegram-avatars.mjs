import fs from 'node:fs'
import path from 'node:path'
import zlib from 'node:zlib'

const SIZE = 1024
const OUTPUT_DIR = path.resolve('services/bot/assets/branding/telegram')

const COLORS = {
  emberGlow: [244, 6, 18],
  emberRed: [229, 9, 20],
  emberDark: [178, 7, 16],
  emberDeep: [97, 10, 20],
  white: [255, 255, 255],
  shadow: [17, 24, 39],
}

const avatars = [
  { key: 'group', filename: 'ember-group-avatar', title: 'Ember Telegram Group' },
  { key: 'channel', filename: 'ember-channel-avatar', title: 'Ember Telegram Channel' },
  { key: 'bot', filename: 'ember-bot-avatar', title: 'Ember Telegram Bot' },
]

const BOT_BADGE_SHIFT_X = 0
const BOT_BADGE_SHIFT_Y = 28

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value))
}

function mix(a, b, t) {
  return a + (b - a) * t
}

function projectT(x, y, x1, y1, x2, y2) {
  const dx = x2 - x1
  const dy = y2 - y1
  const denom = dx * dx + dy * dy
  if (denom === 0) return 0
  return clamp(((x - x1) * dx + (y - y1) * dy) / denom, 0, 1)
}

function insideCircle(x, y, cx, cy, radius) {
  const dx = x - cx
  const dy = y - cy
  return dx * dx + dy * dy <= radius * radius
}

function insideRoundedRect(x, y, left, top, width, height, radius) {
  const right = left + width
  const bottom = top + height
  const cx = clamp(x, left + radius, right - radius)
  const cy = clamp(y, top + radius, bottom - radius)
  const dx = x - cx
  const dy = y - cy
  return dx * dx + dy * dy <= radius * radius
}

function insideArcStroke(x, y, cx, cy, radius, thickness, startAngle, endAngle) {
  const dx = x - cx
  const dy = y - cy
  const dist = Math.sqrt(dx * dx + dy * dy)
  if (dist < radius - thickness / 2 || dist > radius + thickness / 2) {
    return false
  }

  let angle = Math.atan2(dy, dx)
  if (angle < 0) angle += Math.PI * 2
  let start = startAngle
  let end = endAngle
  if (start < 0) start += Math.PI * 2
  if (end < 0) end += Math.PI * 2

  if (start <= end) {
    return angle >= start && angle <= end
  }

  return angle >= start || angle <= end
}

function blend(dst, src) {
  const srcA = src[3]
  const dstA = dst[3]
  const outA = srcA + dstA * (1 - srcA)
  if (outA === 0) return [0, 0, 0, 0]

  return [
    (src[0] * srcA + dst[0] * dstA * (1 - srcA)) / outA,
    (src[1] * srcA + dst[1] * dstA * (1 - srcA)) / outA,
    (src[2] * srcA + dst[2] * dstA * (1 - srcA)) / outA,
    outA,
  ]
}

function paintIf(hit, color, target) {
  return hit ? blend(target, color) : target
}

function sampleBase(x, y) {
  const baseT = projectT(x, y, 140, 120, 888, 912)
  let color = [
    mix(COLORS.emberGlow[0], COLORS.emberDeep[0], baseT),
    mix(COLORS.emberGlow[1], COLORS.emberDeep[1], baseT),
    mix(COLORS.emberGlow[2], COLORS.emberDeep[2], baseT),
    1,
  ]

  if (insideCircle(x, y, 280, 250, 250)) {
    const dx = x - 280
    const dy = y - 250
    const distance = Math.sqrt(dx * dx + dy * dy)
    const alpha = 0.18 * (1 - distance / 250)
    color = blend(color, [255, 255, 255, Math.max(alpha, 0)])
  }

  if (insideCircle(x, y, 820, 870, 360)) {
    const dx = x - 820
    const dy = y - 870
    const distance = Math.sqrt(dx * dx + dy * dy)
    const alpha = 0.18 * (1 - distance / 360)
    color = blend(color, [17, 24, 39, Math.max(alpha, 0)])
  }

  color = paintIf(
    insideRoundedRect(x, y, 212, 188, 600, 600, 184),
    [255, 255, 255, 0.09],
    color,
  )
  color = paintIf(
    insideRoundedRect(x, y, 242, 218, 540, 540, 154),
    [255, 255, 255, 0.05],
    color,
  )

  const eColor = [...COLORS.white, 1]
  color = paintIf(insideRoundedRect(x, y, 328, 250, 96, 428, 48), eColor, color)
  color = paintIf(insideRoundedRect(x, y, 328, 250, 392, 96, 48), eColor, color)
  color = paintIf(insideRoundedRect(x, y, 328, 430, 290, 84, 42), eColor, color)
  color = paintIf(insideRoundedRect(x, y, 328, 582, 392, 96, 48), eColor, color)

  color = paintIf(insideCircle(x, y, 760, 760, 150), [255, 255, 255, 1], color)
  color = paintIf(insideCircle(x, y, 760, 760, 162), [255, 255, 255, 0.14], color)
  color = paintIf(insideCircle(x, y, 760, 760, 126), [229, 9, 20, 0.08], color)

  return color
}

function sampleRole(x, y, role) {
  let color = [0, 0, 0, 0]
  const roleColor = [...COLORS.emberRed, 1]

  if (role === 'group') {
    color = paintIf(insideCircle(x, y, 710, 724, 24), roleColor, color)
    color = paintIf(insideRoundedRect(x, y, 676, 758, 78, 42, 21), roleColor, color)
    color = paintIf(insideCircle(x, y, 796, 736, 30), roleColor, color)
    color = paintIf(insideRoundedRect(x, y, 736, 772, 120, 56, 28), roleColor, color)
  }

  if (role === 'channel') {
    color = paintIf(insideRoundedRect(x, y, 742, 734, 36, 112, 18), roleColor, color)
    color = paintIf(insideCircle(x, y, 760, 700, 24), roleColor, color)
    color = paintIf(insideArcStroke(x, y, 760, 760, 78, 18, 2.09, 4.19), roleColor, color)
    color = paintIf(insideArcStroke(x, y, 760, 760, 78, 18, -1.05, 1.05), roleColor, color)
    color = paintIf(insideArcStroke(x, y, 760, 760, 118, 18, 2.09, 4.19), roleColor, color)
    color = paintIf(insideArcStroke(x, y, 760, 760, 118, 18, -1.05, 1.05), roleColor, color)
  }

  if (role === 'bot') {
    color = paintIf(
      insideRoundedRect(x, y, 680 + BOT_BADGE_SHIFT_X, 702 + BOT_BADGE_SHIFT_Y, 160, 120, 36),
      roleColor,
      color,
    )
    color = paintIf(
      insideRoundedRect(x, y, 748 + BOT_BADGE_SHIFT_X, 654 + BOT_BADGE_SHIFT_Y, 24, 50, 12),
      roleColor,
      color,
    )
    color = paintIf(
      insideCircle(x, y, 760 + BOT_BADGE_SHIFT_X, 638 + BOT_BADGE_SHIFT_Y, 18),
      roleColor,
      color,
    )
    color = paintIf(
      insideCircle(x, y, 724 + BOT_BADGE_SHIFT_X, 760 + BOT_BADGE_SHIFT_Y, 16),
      [255, 255, 255, 1],
      color,
    )
    color = paintIf(
      insideCircle(x, y, 796 + BOT_BADGE_SHIFT_X, 760 + BOT_BADGE_SHIFT_Y, 16),
      [255, 255, 255, 1],
      color,
    )
    color = paintIf(
      insideRoundedRect(x, y, 716 + BOT_BADGE_SHIFT_X, 798 + BOT_BADGE_SHIFT_Y, 88, 14, 7),
      [255, 255, 255, 1],
      color,
    )
  }

  return color
}

function sampleAvatar(x, y, role) {
  let color = sampleBase(x, y)
  color = blend(color, sampleRole(x, y, role))
  return color
}

function writeChunk(type, data) {
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length, 0)
  const typeBuffer = Buffer.from(type)
  const crcBuffer = Buffer.concat([typeBuffer, data])
  const crc = crc32(crcBuffer)
  const crcBytes = Buffer.alloc(4)
  crcBytes.writeUInt32BE(crc >>> 0, 0)
  return Buffer.concat([length, typeBuffer, data, crcBytes])
}

function crc32(buffer) {
  let crc = 0xffffffff
  for (const byte of buffer) {
    crc ^= byte
    for (let i = 0; i < 8; i += 1) {
      const mask = -(crc & 1)
      crc = (crc >>> 1) ^ (0xedb88320 & mask)
    }
  }
  return (crc ^ 0xffffffff) >>> 0
}

function encodePng(width, height, rgba) {
  const scanlines = Buffer.alloc(height * (width * 4 + 1))
  for (let y = 0; y < height; y += 1) {
    const rowOffset = y * (width * 4 + 1)
    scanlines[rowOffset] = 0
    rgba.copy(scanlines, rowOffset + 1, y * width * 4, (y + 1) * width * 4)
  }

  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(width, 0)
  ihdr.writeUInt32BE(height, 4)
  ihdr[8] = 8
  ihdr[9] = 6
  ihdr[10] = 0
  ihdr[11] = 0
  ihdr[12] = 0

  const signature = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])
  const idat = zlib.deflateSync(scanlines, { level: 9 })

  return Buffer.concat([
    signature,
    writeChunk('IHDR', ihdr),
    writeChunk('IDAT', idat),
    writeChunk('IEND', Buffer.alloc(0)),
  ])
}

function renderAvatar(role, size = SIZE, samples = 3) {
  const pixels = Buffer.alloc(size * size * 4)
  const totalSamples = samples * samples

  for (let y = 0; y < size; y += 1) {
    for (let x = 0; x < size; x += 1) {
      let r = 0
      let g = 0
      let b = 0
      let a = 0

      for (let sy = 0; sy < samples; sy += 1) {
        for (let sx = 0; sx < samples; sx += 1) {
          const px = SIZE * (x + (sx + 0.5) / samples) / size
          const py = SIZE * (y + (sy + 0.5) / samples) / size
          const [sr, sg, sb, sa] = sampleAvatar(px, py, role)
          r += sr
          g += sg
          b += sb
          a += sa * 255
        }
      }

      const index = (y * size + x) * 4
      pixels[index] = Math.round(r / totalSamples)
      pixels[index + 1] = Math.round(g / totalSamples)
      pixels[index + 2] = Math.round(b / totalSamples)
      pixels[index + 3] = Math.round(a / totalSamples)
    }
  }

  return pixels
}

function makeRoleGlyph(role) {
  if (role === 'group') {
    return `
      <circle cx="710" cy="724" r="24" fill="#E50914" />
      <rect x="676" y="758" width="78" height="42" rx="21" fill="#E50914" />
      <circle cx="796" cy="736" r="30" fill="#E50914" />
      <rect x="736" y="772" width="120" height="56" rx="28" fill="#E50914" />
    `
  }

  if (role === 'channel') {
    return `
      <circle cx="760" cy="700" r="24" fill="#E50914" />
      <rect x="742" y="734" width="36" height="112" rx="18" fill="#E50914" />
      <path d="M722 694 A78 78 0 0 0 722 826" fill="none" stroke="#E50914" stroke-width="18" stroke-linecap="round" />
      <path d="M798 694 A78 78 0 0 1 798 826" fill="none" stroke="#E50914" stroke-width="18" stroke-linecap="round" />
      <path d="M682 654 A118 118 0 0 0 682 866" fill="none" stroke="#E50914" stroke-width="18" stroke-linecap="round" />
      <path d="M838 654 A118 118 0 0 1 838 866" fill="none" stroke="#E50914" stroke-width="18" stroke-linecap="round" />
    `
  }

  return `
    <g transform="translate(${BOT_BADGE_SHIFT_X} ${BOT_BADGE_SHIFT_Y})">
      <rect x="680" y="702" width="160" height="120" rx="36" fill="#E50914" />
      <rect x="748" y="654" width="24" height="50" rx="12" fill="#E50914" />
      <circle cx="760" cy="638" r="18" fill="#E50914" />
      <circle cx="724" cy="760" r="16" fill="#FFFFFF" />
      <circle cx="796" cy="760" r="16" fill="#FFFFFF" />
      <rect x="716" y="798" width="88" height="14" rx="7" fill="#FFFFFF" />
    </g>
  `
}

function makeSvg(title, role) {
  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" role="img" aria-labelledby="title desc">
  <title id="title">${title}</title>
  <desc id="desc">Ember Telegram ${role} avatar with a unified ember red background, a white E mark, and a role-specific badge.</desc>
  <defs>
    <linearGradient id="bg" x1="140" y1="120" x2="888" y2="912" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#F40612" />
      <stop offset="0.54" stop-color="#C20B18" />
      <stop offset="1" stop-color="#610A14" />
    </linearGradient>
    <radialGradient id="glow" cx="280" cy="250" r="250" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.18" />
      <stop offset="1" stop-color="#FFFFFF" stop-opacity="0" />
    </radialGradient>
    <radialGradient id="shade" cx="820" cy="870" r="360" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#111827" stop-opacity="0.18" />
      <stop offset="1" stop-color="#111827" stop-opacity="0" />
    </radialGradient>
  </defs>

  <rect width="1024" height="1024" fill="url(#bg)" />
  <circle cx="280" cy="250" r="250" fill="url(#glow)" />
  <circle cx="820" cy="870" r="360" fill="url(#shade)" />

  <rect x="212" y="188" width="600" height="600" rx="184" fill="#FFFFFF" fill-opacity="0.09" />
  <rect x="242" y="218" width="540" height="540" rx="154" fill="#FFFFFF" fill-opacity="0.05" />

  <g fill="#FFFFFF">
    <rect x="328" y="250" width="96" height="428" rx="48" />
    <rect x="328" y="250" width="392" height="96" rx="48" />
    <rect x="328" y="430" width="290" height="84" rx="42" />
    <rect x="328" y="582" width="392" height="96" rx="48" />
  </g>

  <circle cx="760" cy="760" r="162" fill="#FFFFFF" fill-opacity="0.14" />
  <circle cx="760" cy="760" r="150" fill="#FFFFFF" />
  <circle cx="760" cy="760" r="126" fill="#E50914" fill-opacity="0.08" />
  ${makeRoleGlyph(role)}
</svg>
`
}

fs.mkdirSync(OUTPUT_DIR, { recursive: true })

for (const avatar of avatars) {
  const svgPath = path.join(OUTPUT_DIR, `${avatar.filename}.svg`)
  const pngPath = path.join(OUTPUT_DIR, `${avatar.filename}.png`)

  fs.writeFileSync(svgPath, makeSvg(avatar.title, avatar.key))

  const pixels = renderAvatar(avatar.key)
  fs.writeFileSync(pngPath, encodePng(SIZE, SIZE, pixels))
}
