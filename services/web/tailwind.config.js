/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        cinema: {
          bg: '#F9FAFB',      // 亮：浅暖灰
          surface: '#FFFFFF', // 亮：纯白
          surfaceLight: '#F3F4F6', // 亮：浅灰
        },
        ember: {
          DEFAULT: '#0F766E', // Deep Ocean Teal
          glow: '#14B8A6',
          dim: 'rgba(15, 118, 110, 0.1)',
          gold: '#F59E0B',
          teal: '#14B8A6',
        },
        text: {
          primary: '#1F2937',   // Dark Gray
          secondary: '#6B7280', // Mid Gray
          muted: '#9CA3AF',     // Light Gray
        }
      },
      fontFamily: {
        sans: ['Plus Jakarta Sans', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'monospace'],
      },
      boxShadow: {
        'soft': '0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -1px rgba(0, 0, 0, 0.03)',
      }
    },
  },
  plugins: [],
  darkMode: 'class',
}
