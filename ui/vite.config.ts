import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";

// https://vite.dev/config/
export default defineConfig({
  plugins: [tsconfigPaths(), react()],
  experimental: {
    renderBuiltUrl(filename: string) {
      return "/ui/" + filename;
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    // Reuses the jsdom environment across test files, which is created once per worker instead of per file.
    pool: 'vmThreads',
    setupFiles: './src/test/setup.ts',
  },
})
