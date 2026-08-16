import type { CapacitorConfig } from "@capacitor/cli";

// Capacitor config for the iOS shell (Android later). Wraps the Vite build
// (`dist/`) in a native WebView — same pattern as korkort-app/terapi-grejen.
// The bundled SPA talks to the user's own tract server via the runtime
// server-address setting (src/api.ts serverBase), or a build-time default:
//   VITE_DEFAULT_SERVER=http://<mac>.local:8080 npm run build
// pre-points a personal build so the device app works with zero setup.
//
// appId is reverse-DNS under a domain we actually control. It used to be
// se.tract.app — .se reverse-DNS for a domain nobody owns, straight out of the
// Capacitor template — which is a claim on someone else's namespace and can be
// taken from us. Vägen made the same move first (korkort-app@3f838f5) and
// clipboard-manager already ships as com.edvinpierre.*, which is also the
// identity behind the shared iCloud container, so this is the house convention
// rather than a fresh invention.
//
// Changed 2026-08-16 while it was still FREE: a bundle identifier is immutable
// once an App Store or Play update channel exists. Tract has not shipped, so the
// window was open; terapi-grejen's closes first and is tracked separately.
// Keep it stable from here.

const config: CapacitorConfig = {
  appId: "com.edvinpierre.tract",
  appName: "Tract",
  webDir: "dist",
  server: {
    androidScheme: "https",
  },
  ios: {
    contentInset: "always",
    limitsNavigationsToAppBoundDomains: false,
    scrollEnabled: true,
    preferredContentMode: "mobile",
  },
  android: {
    allowMixedContent: false,
    captureInput: true,
    webContentsDebuggingEnabled: false,
  },
};

export default config;
