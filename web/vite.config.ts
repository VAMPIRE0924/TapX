import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

const rootDir = fileURLToPath(new URL('.', import.meta.url));

const backendTarget = process.env.TAPX_BACKEND_TARGET || 'http://127.0.0.1:8080';

function sharedVendorChunk(id: string): string | undefined {
  const normalized = id.replace(/\\/g, '/');
  if (!normalized.includes('/node_modules/')) return undefined;
  if (/\/node_modules\/(react|react-dom|scheduler)\//.test(normalized)) return 'vendor-react';
  if (normalized.includes('/node_modules/@ant-design/icons/')) return 'vendor-icons';
  if (
    normalized.includes('/node_modules/@ant-design/cssinjs/')
    || normalized.includes('/node_modules/@ant-design/fast-color/')
    || normalized.includes('/node_modules/antd/es/theme/')
  ) return 'vendor-theme';
  return undefined;
}

export default defineConfig({
  base: './',
  plugins: [react()],
  server: {
    strictPort: false,
    proxy: {
      '^/api(?:/|$)': {
        target: backendTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      input: {
        panel: `${rootDir}index.html`,
        login: `${rootDir}login.html`,
      },
      output: {
        manualChunks: sharedVendorChunk,
      },
    },
  },
});
