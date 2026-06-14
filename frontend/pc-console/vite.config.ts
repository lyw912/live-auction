import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { existsSync, readFileSync } from 'node:fs';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const backendTarget = env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080';
  const mediaTarget = env.LIVE_AUCTION_MEDIAMTX_TARGET || 'http://127.0.0.1:8889';
  const forwardedHost = env.LIVE_AUCTION_PC_FORWARDED_HOST || '106.52.68.95:5277';
  const httpsCert = env.LIVE_AUCTION_HTTPS_CERT || '';
  const httpsKey = env.LIVE_AUCTION_HTTPS_KEY || '';
  const https =
    httpsCert && httpsKey && existsSync(httpsCert) && existsSync(httpsKey)
      ? {
          cert: readFileSync(httpsCert),
          key: readFileSync(httpsKey)
        }
      : undefined;
  const proxy = {
    '/api': {
      target: backendTarget,
      changeOrigin: true,
      headers: {
        'X-Forwarded-Host': forwardedHost
      }
    },
    '/mtx': {
      target: mediaTarget,
      changeOrigin: true,
      rewrite: (path: string) => path.replace(/^\/mtx/, '')
    }
  };

  return {
    plugins: [react(), tailwindcss()],
    build: {
      rollupOptions: {
        output: {
          manualChunks: {
            react: ['react', 'react-dom'],
            icons: ['lucide-react']
          }
        }
      }
    },
    server: {
      https,
      proxy
    },
    preview: {
      https,
      proxy
    }
  };
});
