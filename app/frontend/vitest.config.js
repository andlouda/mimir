import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Run unit tests next to source files. Exclude the Playwright suite —
    // Playwright provides its own runner and would clash with Vitest's test()
    // import otherwise.
    include: ['src/**/*.test.{js,ts}'],
    exclude: ['node_modules/**', 'tests/**', 'frontend/wailsjs/**'],
    environment: 'node',
  },
});
