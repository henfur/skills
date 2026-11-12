import { defineConfig } from "@playwright/test";

// Smoke tests against a real daemon on a scratch home (tests/seed.sh). On NixOS, run them with
// nixpkgs' browsers: PLAYWRIGHT_BROWSERS_PATH=$(nix build --print-out-paths nixpkgs#playwright-driver.browsers)
const port = Number(process.env.PORT || 4799);
export default defineConfig({
  testDir: "tests",
  workers: 1,               // the tests share one daemon and change its state
  timeout: 30_000,
  use: { baseURL: `http://127.0.0.1:${port}`, trace: "retain-on-failure" },
  webServer: {
    command: "bash tests/seed.sh",
    url: `http://127.0.0.1:${port}/api/hub`,
    env: { PORT: String(port) },
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
