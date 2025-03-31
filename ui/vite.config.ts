import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:4040',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
      '/public/fonts': {
        target: 'http://localhost:5173',
        rewrite: (path) => path.replace(/^\/public\/fonts/, '/fonts'),
      },
    },
  },
});
