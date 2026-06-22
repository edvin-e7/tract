import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// base: "./" → assets are referenced relatively, so the build works whether the
// Go binary serves it at / or under a subpath (avoids the absolute-/assets 404).
export default defineConfig({
  base: "./",
  plugins: [react()],
  server: {
    // Dev proxy so the React dev server talks to the Go backend on :8080.
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
