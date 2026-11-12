# lacarta: tech stack

## 1. Choices

| Layer | Choice | Version | Why this one | Rejected alternatives |
| --- | --- | --- | --- | --- |
| Language | Go | 1.26 (module `go 1.26`) | One static binary per host, VM or container; easy long-running hooks and HTTP; `go:embed` for the console and skill | Python (the prototype: needs an interpreter, no single artifact), Rust (slower to iterate, no gain here) |
| Framework (backend) | Standard library only | | `net/http`, `encoding/json`, `syscall.Flock` cover everything; no supply chain | Gin/Echo/Chi (unneeded), cobra (the CLI contract mirrors argparse; a 280-line parser is enough) |
| Framework (console) | Svelte 5 with Vite, no SvelteKit | Svelte 5.57.3, Vite 6.4.6 | Reactive views replace the prototype's re-render workarounds; small output; Go serves pages and API | React (heavier), SvelteKit (routing and SSR not needed), plain HTML strings (the prototype; outgrown) |
| Build tool (console) | Bun | 1.4.2 (nixpkgs) | Fast install and build; only at build time | npm/pnpm (fine too: the output is static either way) |
| Storage | JSON files under `flock` | | Works with the daemon down; software state is committable; shared with the prototype | SQLite (not diffable or committable, harder interop) |
| Markdown, sanitising, diagrams | marked 12.0.2, DOMPurify 3.1.6, Mermaid 10.9.1 | vendored | The console renders specs and question bodies offline; DOMPurify sanitises everything marked produces; Mermaid loads lazily (3 MB) | Rendering markdown in Go (diagrams still need Mermaid in the browser) |
| Testing | `go test`; golden parity suite; Playwright | Playwright 1.64.1 (= nixpkgs `playwright-driver`) | | Scenario recordings make behaviour reviewable as text diffs | Unit tests only (miss end-to-end contract details) |
| Build / packaging | `go build`; release binaries for linux and darwin on amd64 and arm64 (phase 4) | | One file to copy into any environment | Containers (lacarta runs beside agents, not as a service) |
| Runtime | Linux and macOS; Unix advisory locks | | Where agent CLIs run | Windows (no `flock`; out of scope) |

## 2. Dependencies

Go module: no third-party dependencies. Keep it that way unless a dependency removes real risk; if fsnotify is ever added for AD-5, pin it and say why in the README.

Console: `svelte` 5.57.3, `vite` 6.4.6, `@sveltejs/vite-plugin-svelte` 5.1.1, `marked` 12.0.2, `dompurify` 3.1.6, `mermaid` 10.9.1 (loaded only on pages with a diagram), `@playwright/test` 1.64.1. Exact versions in `package.json`, `bun.lock` committed. DOMPurify and marked have newer releases; upgrading is fine once the Playwright suite passes.

External programs: none at runtime. Tests use `python3` while the prototype exists (golden recordings, `TestAPIParity`).

## 3. Tooling

- Format and vet: `gofmt -w .`, `go vet ./...`.
- Lint: `golangci-lint run ./...` (installed on the dev machine). Unchecked writes to stdout and deferred `Close` are accepted; anything else should be fixed.
- Tests: `go test ./...`. Re-record goldens from the prototype: `GOLDEN_IMPL=python GOLDEN_RECORD=1 go test ./internal/golden`; after phase 4, re-record with Go (`GOLDEN_RECORD=1 go test ./internal/golden`) and review the diff.
- Console: `bun install`, `bun run build` (output `web/dist/`, commit it), `bun run build --watch` with a daemon started with `LACARTA_WEB_DIR=web/dist`, `bunx playwright test` (see the README for the NixOS browser path).

## 4. Environments

- Development: NixOS, Go 1.26 from the system; Bun through `nix shell nixpkgs#bun` (not installed globally). A `flake.nix` or `shell.nix` providing `go`, `bun` and `golangci-lint` would make this one command.
- The user's machine runs the Go binary as `~/.local/bin/lacarta` with hooks installed in `~/.claude/settings.json` and the console on `127.0.0.1:4747`. Never test against that home or port: use a temporary `LACARTA_HOME`, `CLAUDE_CONFIG_DIR` and a free port.
- The machine's `python3` is a wrapper that needs the real `HOME`; tests isolate lacarta with `LACARTA_HOME` instead of `HOME`, and set `LANG=C.UTF-8`.

## 5. Constraints

- The binary must stay self-contained: embedded console, vendored scripts, embedded skill.
- The console must work offline: no CDN at runtime (Google Fonts are optional styling; the page must look right without them).
- CLI output, hook JSON and state files are contracts (functional NFR-1, NFR-2).
- Python 3 remains a test-time dependency only until phase 4 retires the prototype.
