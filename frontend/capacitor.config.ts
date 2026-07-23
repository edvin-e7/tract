import type { CapacitorConfig } from "@capacitor/cli";

// Capacitor config for the iOS shell (Android later). Wraps the Vite build
// (`dist/`) in a native WebView — same pattern as korkort-app/terapi-grejen.
// The bundled SPA talks to the user's own tract server via the runtime
// server-address setting (src/api.ts serverBase), or a build-time default:
//   VITE_DEFAULT_SERVER=http://<mac>.local:8080 npm run build
// pre-points a personal build so the device app works with zero setup.
//
// appId uses reverse-DNS under a made-up domain; swap if Edvin registers a
// real one. Keep it stable — changing appId breaks App Store update channels.

const config: CapacitorConfig = {
  appId: "se.tract.app",
  appName: "Tract",
  webDir: "dist",
  server: {
    androidScheme: "https",
  },
  ios: {
    contentInset: "always",
    limitsNavigationsToAppBoundDomains: true,
  },
  android: {
    allowMixedContent: false,
    captureInput: true,
    webContentsDebuggingEnabled: false,
  },
};

export default config;
