/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Build-time default server origin for native-shell builds pre-pointed at a
   * personal server (e.g. http://my-mac.local:8080). Empty/unset = same-origin. */
  readonly VITE_DEFAULT_SERVER?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
