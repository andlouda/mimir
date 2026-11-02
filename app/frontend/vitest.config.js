import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  // Component tests mount .svelte files (per-file `@vitest-environment jsdom`).
  plugins: [svelte({ hot: false })],
  // Resolve Svelte's browser build, not the server one, so mount() works.
  resolve: { conditions: ['browser'] },
  test: {
    // Run unit tests next to source files. Exclude the Playwright suite —
    // Playwright provides its own runner and would clash with Vitest's test()
    // import otherwise.
    include: ['src/**/*.test.{js,ts}'],
    exclude: ['node_modules/**', 'tests/**', 'frontend/wailsjs/**'],
    environment: 'node',
  },
});
