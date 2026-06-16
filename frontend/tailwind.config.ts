import type { Config } from 'tailwindcss'
const config: Config = {
  content: ['./app/**/*.{ts,tsx}'],
  theme: { extend: { fontFamily: { sans: ['Inter', 'system-ui', 'sans-serif'], serif: ['Cormorant Garamond', 'Georgia', 'serif'] }, colors: { cream: '#F2EFE6', copper: '#C38133', dark: '#231F20' } } },
}
export default config
