/// <reference types="vite/client" />

// Build-time stamp injected by the Dockerfile web stage (see Dockerfile +
// .gitlab-ci.yml). On `npm run dev` they're undefined and the Footer falls
// back to "dev" / "local" / today's date.
interface ImportMetaEnv {
  readonly VITE_APP_VERSION?: string;
  readonly VITE_APP_COMMIT?: string;
  readonly VITE_APP_BUILD_DATE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
