import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@components': fileURLToPath(
        new URL('./src/components', import.meta.url),
      ),
      '@hooks': fileURLToPath(new URL('./src/hooks', import.meta.url)),
      '@api': fileURLToPath(new URL('./src/api', import.meta.url)),
      '@lib': fileURLToPath(new URL('./src/lib', import.meta.url)),
    },
  },
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: fileURLToPath(new URL('./index.html', import.meta.url)),
        admin: fileURLToPath(new URL('./src/admin/admin.tsx', import.meta.url)),
      },
      output: {
        // Admin bundle is loaded by name from Go-rendered HTML templates
        // (pkg/operations/v2/querydiagnostics/*.gohtml), so its filename must
        // be stable. Other entries keep content hashes for cache busting.
        entryFileNames: (chunk) =>
          chunk.name === 'admin'
            ? 'assets/admin.js'
            : 'assets/[name]-[hash].js',
        assetFileNames: (asset) => {
          const names = [asset.name, ...(asset.names ?? [])].filter(
            (n): n is string => typeof n === 'string',
          );
          if (
            names.some((n) => n === 'admin.css' || n.endsWith('/admin.css'))
          ) {
            return 'assets/admin.css';
          }
          return 'assets/[name]-[hash][extname]';
        },
      },
    },
  },
  server: {
    proxy: {
      '/querier.v1.QuerierService': 'http://localhost:4040',
    },
  },
});
