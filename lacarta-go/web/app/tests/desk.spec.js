import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

// The seed's review desk: one merge request reviewed in shadow mode, its plan waiting for the user.
// GitLab is the Go tests' fake glab (tests/.run/gitlab.json).
const ids = () => JSON.parse(readFileSync(new URL("./.run/ids.json", import.meta.url)));
const gitlab = () => JSON.parse(readFileSync(ids().gitlab));

test.beforeEach(async ({ page }) => {
  const errors = [];
  page.on("console", m => { if (m.type() === "error" && !/status of 409/.test(m.text())) errors.push(m.text()); });
  page.on("pageerror", e => errors.push(String(e)));
  page.errors = errors;
});
test.afterEach(async ({ page }) => {
  expect(page.errors || [], "browser errors").toEqual([]);
});

test("desk views render", async ({ page }) => {
  for (const view of ["overview", "chat", "questions", "queue", "desk", "decisions", "activity"]) {
    await page.goto(`/#/p/${ids().desk}/${view}`);
    await expect(page.locator(".ptabs a[aria-current=page]")).toBeVisible();
    await expect(page.locator("main h1").first()).toBeVisible();
  }
  await page.goto(`/#/p/${ids().desk}/overview`);
  await expect(page.getByText("Drafts for you")).toBeVisible();
  await page.goto(`/#/p/${ids().desk}/desk`);
  await expect(page.getByRole("radio", { name: "Shadow" })).toBeChecked();
});

test("the user edits a draft and publishes it", async ({ page }) => {
  expect(gitlab().mrs[0].notes || []).toHaveLength(0);   // shadow: nothing posted on its own
  await page.goto(`/#/p/${ids().desk}/queue`);
  const mr = page.locator("section.card", { hasText: "acme/billing!7" });
  await expect(mr.getByText("Download has no test").first()).toBeVisible();
  await mr.getByRole("button", { name: "Edit" }).first().click();
  await mr.getByRole("textbox").fill("Edited in the console: please add a test for Download.");
  await mr.getByRole("button", { name: "Save" }).click();
  await expect(mr.getByText("edited by you")).toBeVisible();
  await mr.getByRole("button", { name: "Publish the waiting steps" }).click();
  await expect(mr).toHaveCount(0);                      // done: it leaves the "In progress" list
  await page.getByLabel("Everything").check();
  await expect(mr.getByText("published", { exact: true })).toBeVisible();
  const notes = gitlab().mrs[0].notes;
  expect(notes.length).toBeGreaterThanOrEqual(2);
  expect(notes.some(n => n.body.includes("Edited in the console"))).toBe(true);
  expect(notes.every(n => n.author === "review-bot" && n.body.startsWith("<!-- lacarta:review"))).toBe(true);
});
