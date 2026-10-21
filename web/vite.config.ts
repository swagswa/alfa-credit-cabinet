import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    // api в compose слушает 8080, cookie сессии ходит через тот же origin
    proxy: { '/api': 'http://localhost:8080' },
  },
});
