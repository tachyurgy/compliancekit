/// <reference types="vite/client" />

// Tell TS about Vite's `?raw` query suffix for static-string imports of
// Markdown / SQL / etc. used by the legal-doc bundler in
// pages/onboarding/OnboardingLegal.tsx.
declare module '*.md?raw' {
  const content: string;
  export default content;
}

// Augment ImportMetaEnv with the env vars we actually read in app code so
// `import.meta.env.VITE_*` has correct types instead of the any-default.
interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  readonly VITE_FRONTEND_BASE_URL?: string;
  readonly VITE_SENTRY_DSN?: string;
  readonly VITE_DEMO_MODE?: string;
  readonly MODE: 'development' | 'production' | 'test';
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly SSR: boolean;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
