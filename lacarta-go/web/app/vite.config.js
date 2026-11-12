import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// The build lands in ../dist, which the Go binary embeds. During development, point a daemon
// at it with LACARTA_WEB_DIR=web/dist and run `bun run build --watch`: no Go rebuild needed.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: "../dist",
    emptyOutDir: true,
    assetsDir: "assets",
    chunkSizeWarningLimit: 4000, // mermaid is large; it only loads on pages with a diagram
  },
});
