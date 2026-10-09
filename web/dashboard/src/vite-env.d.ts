/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** API root: "/api" (default) or a full URL. */
  readonly VITE_API_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
