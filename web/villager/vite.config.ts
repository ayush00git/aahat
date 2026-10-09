import { readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { join } from 'node:path';
import { defineConfig, loadEnv, type Plugin } from 'vite';

// Files from public/ that belong to the app shell.
const PUBLIC_SHELL = [
  'manifest.webmanifest',
  'favicon.svg',
  'icons/icon-192.png',
  'icons/badge-96.png',
];

const SW_TEMPLATE = new URL('./sw/sw.js', import.meta.url);

// serviceWorker writes dist/sw.js from sw/sw.js, filling in the list of files
// the first screen needs (entry chunk, its static imports and CSS, index.html,
// icons). The lazily loaded map chunks are cached at runtime when first used.
// In dev it serves the same worker with an empty precache list.
function serviceWorker(): Plugin {
  const render = (files: string[], dev: boolean) => {
    const version = createHash('sha256').update(files.join('\n')).digest('hex').slice(0, 12);
    return readFileSync(SW_TEMPLATE, 'utf8')
      .replace('__PRECACHE__', JSON.stringify(files))
      .replace('__VERSION__', JSON.stringify(dev ? 'dev' : version))
      .replace('__DEV__', JSON.stringify(dev));
  };
  return {
    name: 'aahat-service-worker',
    configureServer(server) {
      server.middlewares.use('/sw.js', (_req, res) => {
        res.setHeader('Content-Type', 'text/javascript; charset=utf-8');
        res.setHeader('Cache-Control', 'no-cache');
        res.end(render([], true));
      });
    },
    writeBundle(options, bundle) {
      const files = new Set<string>(['./', 'index.html', ...PUBLIC_SHELL]);
      const visit = (name: string) => {
        const chunk = bundle[name];
        if (!chunk || files.has(name)) return;
        files.add(name);
        if (chunk.type !== 'chunk') return;
        const meta = (chunk as { viteMetadata?: { importedCss?: Set<string>; importedAssets?: Set<string> } }).viteMetadata;
        meta?.importedCss?.forEach((f) => files.add(f));
        meta?.importedAssets?.forEach((f) => files.add(f));
        chunk.imports.forEach(visit); // static imports only, not dynamic ones
      };
      for (const [name, chunk] of Object.entries(bundle)) {
        if (chunk.type === 'chunk' && chunk.isEntry) visit(name);
      }
      writeFileSync(join(options.dir ?? 'dist', 'sw.js'), render([...files], false));
    },
  };
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = env.AAHAT_API_TARGET || 'http://127.0.0.1:8080';
  return {
    base: './',
    oxc: { jsx: { runtime: 'automatic', importSource: 'preact' } },
    plugins: [serviceWorker()],
    build: {
      target: 'es2020',
      // MapLibre is >200 KB on its own; it is lazy-loaded, so don't warn.
      chunkSizeWarningLimit: 1200,
    },
    server: {
      proxy: {
        '/api': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
    preview: {
      proxy: {
        '/api': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
        },
      },
    },
  };
});
