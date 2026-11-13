import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

// The seed's security audit: findings submitted, waiting for the auditor's review. The seed
// skips it on machines without the security-audit skill, node or bubblewrap.
const ids = () => JSON.parse(readFileSync(new URL("./.run/ids.json", import.meta.url)));

test.beforeEach(async ({ page }) => {
  test.skip(!ids().audit, "this machine can't run audits (see tests/.run/preflight.txt)");
  const errors = [];
  page.on("console", m => {
    if (m.type() === "error" && !/status of 409/.test(m.text())) errors.push(m.text());
  });
  page.on("pageerror", e => errors.push(String(e)));
  page.errors = errors;
});
test.afterEach(async ({ page }) => {
  expect(page.errors || [], "browser errors").toEqual([]);
});

test("audit views render", async ({ page }) => {
  for (const view of ["overview", "chat", "questions", "coverage", "findings", "report", "plugins", "decisions", "activity"]) {
    await page.goto(`/#/p/${ids().audit}/${view}`);
    await expect(page.locator(".ptabs a[aria-current=page]")).toBeVisible();
    await expect(page.locator("main h1").first()).toBeVisible();
  }
  await page.goto(`/#/p/${ids().audit}/overview`);
  await expect(page.locator(".step.current")).toContainText("Your review");
  await page.goto(`/#/p/${ids().audit}/coverage`);
  await page.locator(".coverage .cell").first().click();
  await expect(page.getByText("Update order")).toBeVisible();
  await page.goto(`/#/p/${ids().audit}/plugins`);
  const skill = page.getByRole("checkbox", { name: "Enable Cloudflare security-audit skill" });
  await expect(skill).toBeChecked();
  await expect(skill).toBeDisabled();   // required: can't be turned off
});

test("the auditor reviews, approves and gets a signed report", async ({ page, request }) => {
  await page.goto(`/#/p/${ids().audit}/report`);
  await expect(page.getByRole("button", { name: "Approve and sign the report" })).toHaveCount(0);  // findings not reviewed

  await page.goto(`/#/p/${ids().audit}/findings`);
  const conf = page.locator("article.finding", { hasText: "Any user can update any order" });
  await conf.getByRole("button", { name: /Change the severity/ }).click();
  await conf.getByLabel("Severity").selectOption("medium");
  await expect(conf.getByRole("button", { name: "Save review" })).toBeDisabled();  // a reason is required
  await conf.getByLabel(/Reason/).fill("Orders hold no payment data.");
  await conf.getByRole("button", { name: "Save review" }).click();
  await expect(conf.getByText("severity changed from High to Medium")).toBeVisible();

  await page.getByRole("tab", { name: /Needs validation/ }).click();
  const nv = page.locator("article.finding", { hasText: "Unbounded body size" });
  await nv.getByRole("button", { name: /Keep as needs validation/ }).click();
  await nv.getByRole("button", { name: "Save review" }).click();
  await expect(nv.getByText("Your review:")).toBeVisible();

  await page.goto(`/#/p/${ids().audit}/report`);
  await expect(page.getByText("set your name as the auditor")).toBeVisible();
  await page.goto("/#/settings");
  await page.getByLabel(/Your name, as it appears/).fill("Ada Auditor");
  await page.getByRole("button", { name: "Save name" }).click();
  await expect(page.getByText("Saved.")).toBeVisible();

  await page.goto(`/#/p/${ids().audit}/report`);
  const draft = page.locator("section.card", { hasText: "DRAFT: not approved" });
  await expect(draft).toContainText("performed with AI assistance");
  await expect(draft).toContainText("Severity changed from High to Medium");
  const approve = page.getByRole("button", { name: "Approve and sign the report" });
  await expect(approve).toBeDisabled();
  await page.getByRole("checkbox", { name: /I, Ada Auditor, reviewed this report/ }).check();
  await approve.click();
  await expect(page.getByRole("heading", { name: "Approved versions" })).toBeVisible();
  await expect(page.getByText(/approved by Ada Auditor/)).toBeVisible();

  const html = await page.getByRole("link", { name: "Open (print to PDF)" }).getAttribute("href");
  const r = await request.get(html);
  expect(r.ok()).toBeTruthy();
  const body = await r.text();
  expect(body).toContain("AI-ASSISTED AUDIT");
  expect(body).toContain("Reviewed and approved by Ada Auditor");
  expect(r.headers()["content-security-policy"]).toContain("default-src 'none'");
  await page.goto(`/#/p/${ids().audit}/overview`);
  await expect(page.locator(".step.current, .step.done").last()).toContainText("Done");
});
