import { test, expect } from "@playwright/test";
import { readFileSync, appendFileSync } from "node:fs";
import { execFileSync } from "node:child_process";

// The seed script's projects: a task with a question, an ended task, a software project with a
// blocking finding and a diagram.
const ids = () => JSON.parse(readFileSync(new URL("./.run/ids.json", import.meta.url)));
const lacarta = (...args) => execFileSync(ids().lacarta, args, {
  env: { ...process.env, LACARTA_HOME: ids().home, LACARTA_NO_DAEMON: "1" }, encoding: "utf8",
});

// Every test fails on a console error or a page error: that includes CSP violations. A 409 is a
// gate refusing an action, which the page shows; the browser logs it as an error anyway.
test.beforeEach(async ({ page }) => {
  const errors = [];
  page.on("console", m => {
    if (m.type() === "error" && !/status of 409/.test(m.text())) errors.push(m.text());
  });
  page.on("pageerror", e => errors.push(String(e)));
  page.errors = errors;
});
test.afterEach(async ({ page }) => {
  expect(page.errors, "browser errors").toEqual([]);
});

test("home lists the projects and what waits for the user", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Projects" })).toBeVisible();
  await expect(page.locator(".project-card .pname", { hasText: "Fix the login bug" })).toBeVisible();
  await expect(page.locator(".project-card .pname", { hasText: "Shop" })).toBeVisible();
  await expect(page.getByRole("heading", { name: /Waiting for you · 2/ })).toBeVisible();
  await expect(page).toHaveTitle(/^\(2\) Lacarta console/);
  await expect(page.locator(".live")).toContainText("updated");
});

test("answer a question", async ({ page }) => {
  await page.goto(`/#/p/${ids().task}/questions`);
  const card = page.locator("article.q", { hasText: "Which fix?" });
  await expect(card.locator(".choices-detailed .help").first()).toHaveText("Small change, quick to review");
  await card.getByRole("button", { name: /^Rewrite/ }).click();
  await expect(card.getByRole("button", { name: /^Rewrite/ })).toHaveAttribute("aria-pressed", "true");
  await card.locator("textarea").fill("cleaner is worth it");
  await card.getByRole("button", { name: "Send answer" }).click();
  await expect(page.locator("article.q")).toHaveCount(0);
  await expect(page.getByText("Answer: Rewrite")).toBeVisible();
  expect(lacarta("--project", ids().task, "decisions")).toContain("- Decision: Rewrite");
});

test("triage: option meanings, Talk about this, a reason to accept the risk", async ({ page }) => {
  await page.goto(`/#/p/${ids().shop}/questions`);
  const card = page.locator("article.q.blocking");
  await expect(card.getByRole("heading", { name: /No auth model/ })).toBeVisible();
  await expect(card.getByText("How it goes wrong")).toBeVisible();
  await card.getByRole("button", { name: /^Accept the risk/ }).click();
  await expect(card.locator("label.field")).toHaveText("Why you accept this risk (required)");
  await card.getByRole("button", { name: "Send answer" }).click();
  await expect(card.locator(".err")).toHaveText("Accepting a high or critical risk needs a written rationale.");

  await card.getByRole("button", { name: /^Talk about this/ }).click();
  await card.locator("textarea").fill("How likely is this?");
  await card.getByRole("button", { name: /^Send to / }).click();
  await expect(card.locator(".thread")).toContainText("How likely is this?");
  await expect(card.locator(".thread")).toContainText("to reply…");
  await expect(card.getByRole("button", { name: "Send answer" })).toBeVisible();  // back to answering
});

test("chat shows the transcript and queues a message", async ({ page }) => {
  await page.goto(`/#/p/${ids().task}/chat`);
  await expect(page.locator(".chat-log")).toContainText("Fix the login bug");
  await expect(page.locator(".chat-log strong", { hasText: "auth.py" })).toBeVisible();   // markdown
  await expect(page.locator(".chat-log .tools")).toContainText("2 tool calls");
  await page.locator("#chat-input").fill("Also check logout");
  await page.locator("#chat-input").press("Control+Enter");
  await expect(page.locator(".chat-log")).toContainText("Also check logout");
  await expect(page.locator(".chat-log")).toContainText("queued");
  await expect(page.locator(".ptabs")).toContainText("1 queued");
});

test("settings: change a model", async ({ page }) => {
  await page.goto("/#/settings");
  const select = page.getByLabel("Model for Security review");
  await select.selectOption("haiku");
  await expect(page.getByText("Unsaved")).toBeVisible();
  await page.getByRole("button", { name: "Save models" }).click();
  await expect(page.getByText("Saved.")).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Model for Security review")).toHaveValue("haiku");
  expect(lacarta("--root", ids().shopRoot, "models")).toMatch(/security_review\s+haiku/);
});

test("archive an ended task", async ({ page }) => {
  await page.goto("/");
  const card = page.locator(".project-card", { has: page.locator(`a[href="#/p/${ids().done}/overview"]`) });
  await card.getByRole("button", { name: "Archive" }).click();
  await expect(page.getByRole("button", { name: /Show archived · 1/ })).toBeVisible();
  await expect(card).toHaveCount(0);
});

test("live: a question asked from the CLI appears without reload, with a toast", async ({ page }) => {
  await page.goto(`/#/p/${ids().shop}/overview`);
  await expect(page.getByRole("heading", { name: "Overview" })).toBeVisible();
  await page.waitForTimeout(1000);  // the first hub load is never announced
  lacarta("--root", ids().shopRoot, "ask", "--title", "Ship on Friday?");
  await expect(page.locator(".toast")).toContainText("Ship on Friday?", { timeout: 5000 });
  await expect(page.locator(".agent-card.asking")).toContainText("Ship on Friday?");
  await page.locator(".toast").getByRole("link", { name: "Answer" }).click();
  await expect(page).toHaveURL(/\/questions$/);
  await expect(page.locator(".toast")).toHaveCount(0);
});

test("specs render markdown and Mermaid diagrams", async ({ page }) => {
  await page.goto(`/#/p/${ids().shop}/specs`);
  await page.getByRole("tab", { name: /Architecture/ }).click();
  await expect(page.locator("figure.diagram svg")).toBeVisible({ timeout: 10_000 });
  await expect(page.locator("figure.diagram")).toContainText("Browser");
  // The specs read as documentation; the specification review is under Phases & reviews.
  await expect(page.getByText("Independent specification review")).toHaveCount(0);
  await page.goto(`/#/p/${ids().shop}/phases`);
  await expect(page.getByRole("heading", { name: "Independent specification review" })).toBeVisible();
  await expect(page.getByText("No auth model")).toBeVisible();
});

test("every view renders", async ({ page }) => {
  for (const view of ["overview", "chat", "questions", "specs", "phases", "findings", "decisions", "activity"]) {
    await page.goto(`/#/p/${ids().shop}/${view}`);
    await expect(page.locator(".ptabs a[aria-current=page]")).toBeVisible();
    await expect(page.locator("main h1").first()).toBeVisible();   // specs contain their own h1
  }
  for (const view of ["overview", "chat", "questions", "activity"]) {
    await page.goto(`/#/p/${ids().task}/${view}`);
    await expect(page.locator("main h1")).toBeVisible();
  }
  await page.goto("/#/p/nope/overview");
  await expect(page.getByText("No project nope.")).toBeVisible();
});

test("dark theme and phone width", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 800 });
  await page.goto(`/#/p/${ids().shop}/overview`);
  await page.getByRole("button", { name: "Switch theme" }).click();
  const theme = await page.evaluate(() => document.documentElement.dataset.theme);
  expect(["dark", "light"]).toContain(theme);
  for (const view of ["overview", "questions", "findings", "chat"]) {
    await page.goto(`/#/p/${ids().shop}/${view}`);
    await expect(page.locator("main h1")).toBeVisible();
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${view} scrolls sideways at 375 px`).toBeLessThanOrEqual(0);
  }
  await page.reload();
  expect(await page.evaluate(() => document.documentElement.dataset.theme)).toBe(theme);  // remembered
});

// Last: it adds a message to the task's transcript.
test("the chat wraps text and scrolls code blocks on their own", async ({ page }) => {
  const long = "x".repeat(400);
  const text = `A long word ${long} and \`inline_${long}\`\n\n\`\`\`\nconst v = '${long}';\n\`\`\`\n\n| a | b |\n|---|---|\n| ${long} | y |`;
  appendFileSync(new URL("./.run/claude/projects/x/s1.jsonl", import.meta.url), JSON.stringify({
    type: "assistant", timestamp: "2026-11-12T01:01:00Z",
    message: { content: [{ type: "text", text }, { type: "tool_use", name: "Bash", input: { command: `echo ${long}`, description: `Print ${long}` } }] },
  }) + "\n");
  for (const width of [1280, 375]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/#/p/${ids().task}/chat`);
    await expect(page.locator("#chat-log pre")).toBeVisible();
    const r = await page.evaluate(() => {
      const log = document.getElementById("chat-log"), pre = log.querySelector("pre");
      return { log: log.scrollWidth - log.clientWidth, page: document.documentElement.scrollWidth - innerWidth, pre: pre.scrollWidth > pre.clientWidth };
    });
    expect(r).toEqual({ log: 0, page: 0, pre: true });
  }
});
