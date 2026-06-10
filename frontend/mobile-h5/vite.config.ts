import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const backendTarget = env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080';
  const proxy = {
    '/api': {
      target: backendTarget,
      changeOrigin: true
    },
    '/ws': {
      target: backendTarget,
      changeOrigin: false,
      ws: true
    }
  };

  return {
    plugins: [react()],
    server: {
      proxy
    },
    preview: {
      proxy
    }
  };
});
