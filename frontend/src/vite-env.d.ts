/// <reference types="vite/client" />

// Augment ImportMetaEnv with the env vars we actually read in app code so
// `import.meta.env.VITE_*` has correct types instead of the any-default.
interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  readonly VITE_FRONTEND_BASE_URL?: string;
  readonly VITE_SENTRY_DSN?: string;
  readonly MODE: 'development' | 'production' | 'test';
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly SSR: boolean;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
