import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  build: {
    // Never inline assets as data: URLs: the CSP allows fonts from
    // 'self' only, and the small font subsets (< 4 KB) were being
    // inlined and blocked, so those glyph ranges fell back to the
    // system font.
    assetsInlineLimit: 0,
  },
})
