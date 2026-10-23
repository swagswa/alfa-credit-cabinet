import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { mockApi } from './mock-api';

export default defineConfig(({ mode }) => ({
  cacheDir: process.env.VITE_CACHE_DIR ?? 'node_modules/.vite',
  plugins: [react(), ...(mode === 'mock' ? [mockApi()] : [])],
  server: {
    // api в compose слушает 8080, cookie сессии ходит через тот же origin
    proxy: { '/api/samples': 'http://localhost:8081', '/api': 'http://localhost:8080' },
  },
}));
