import { defineConfig } from "vite";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [wails("./bindings")],
  // Wails' asset proxy dials 127.0.0.1; Vite 8 binds "localhost" to ::1 only on some Windows setups.
  server: { host: "127.0.0.1" },
});
