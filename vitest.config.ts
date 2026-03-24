import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['lib/pebble/__tests__/**/*.test.ts'],
    environment: 'node',
  },
});
