import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath, URL } from 'node:url';
import type { Plugin } from 'vite';

// @grafana/flamegraph hardcodes asset paths with a /public prefix (designed
// for the Grafana app server). This middleware strips that prefix so requests
// resolve to the correct Vite-served paths.
function grafanaPublicPathPlugin(): Plugin {
  const rewrite = (req: { url?: string }, _res: unknown, next: () => void) => {
    if (req.url?.startsWith('/public/')) {
      req.url = req.url.slice('/public'.length);
    }
    next();
  };
  return {
    name: 'grafana-public-path-rewrite',
    configureServer(server) {
      server.middlewares.use(rewrite);
    },
    configurePreviewServer(server) {
      server.middlewares.use(rewrite);
    },
  };
}

export default defineConfig({
  plugins: [react(), grafanaPublicPathPlugin()],
  resolve: {
    alias: {
      '@components': fileURLToPath(
        new URL('./src/components', import.meta.url),
      ),
      '@hooks': fileURLToPath(new URL('./src/hooks', import.meta.url)),
      '@api': fileURLToPath(new URL('./src/api', import.meta.url)),
    },
  },
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/querier.v1.QuerierService': 'http://localhost:4040',
    },
  },
});
