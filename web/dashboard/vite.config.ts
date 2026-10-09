import { defineConfig, loadEnv } from "vite";
import preact from "@preact/preset-vite";

// In dev, "/api/*" is proxied to the Go API (path prefix stripped), so the
// dashboard and API share an origin. Override the target with
// AAHAT_API_TARGET, e.g. AAHAT_API_TARGET=http://127.0.0.1:8080 npm run dev.
//
// Production builds are served under /officials/ (Caddy strips the prefix and
// serves dist/), with the API at /api on the same host. Override the base with
// DASHBOARD_BASE=/ npm run build.
export default defineConfig(({ command, mode, isPreview }) => {
  const env = loadEnv(mode, ".", "");
  const target = env.AAHAT_API_TARGET || "http://127.0.0.1:8081";
  const base = command === "build" || isPreview ? env.DASHBOARD_BASE || "/officials/" : "/";
  return {
    base,
    plugins: [preact()],
    server: {
      proxy: {
        "/api": {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ""),
        },
      },
    },
    preview: {
      proxy: {
        "/api": {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ""),
        },
      },
    },
    build: {
      target: "es2022",
      chunkSizeWarningLimit: 1200,
      rollupOptions: {
        output: { manualChunks: { maplibre: ["maplibre-gl"] } },
      },
    },
  };
});
