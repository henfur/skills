import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

// The seed's incident case: a timeline, one investigation, a containment action and a situation
// report, the last two waiting for the lead.
const ids = () => JSON.parse(readFileSync(new URL("./.run/ids.json", import.meta.url)));
const sitrep = () => readFileSync(`${ids().caseRoot}/reports/sitrep-001.md`, "utf8");

test.beforeEach(async ({ page }) => {
  const errors = [];
  page.on("console", m => { if (m.type() === "error" && !/status of 409/.test(m.text())) errors.push(m.text()); });
  page.on("pageerror", e => errors.push(String(e)));
  page.errors = errors;
});
test.afterEach(async ({ page }) => {
  expect(page.errors || [], "browser errors").toEqual([]);
});

test("incident views render", async ({ page }) => {
  for (const view of ["overview", "chat", "questions", "timeline", "investigations", "actions", "evidence", "reports", "decisions", "activity"]) {
    await page.goto(`/#/p/${ids().case}/${view}`);
    await expect(page.locator(".ptabs a[aria-current=page]")).toBeVisible();
    await expect(page.locator("main h1").first()).toBeVisible();
  }
  await page.goto(`/#/p/${ids().case}/overview`);
  await expect(page.getByText("Identify & contain")).toBeVisible();
  await expect(page.getByText("Time to detect")).toBeVisible();
  await page.goto(`/#/p/${ids().case}/timeline`);
  const rows = page.locator("table.data tbody tr");
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(1)).toContainText("Credentials entered");   // in the order things happened
  await expect(rows.nth(1)).toContainText("Suspected");
  await page.goto(`/#/p/${ids().case}/evidence`);
  await expect(page.getByText("hxxps://login[.]example[.]net/x")).toBeVisible();   // never a live link
  await expect(page.getByText("evidence/EV-1.md")).toBeVisible();
});

test("the lead approves an action, then edits and issues a situation report", async ({ page }) => {
  await page.goto(`/#/p/${ids().case}/actions`);
  const act = page.locator("section.card", { hasText: "Reset the password of j.doe" });
  await expect(act.getByText("waits for you")).toBeVisible();
  await expect(act.getByText("The user is logged out everywhere.")).toBeVisible();
  await act.getByRole("button", { name: "Approve" }).click();
  await expect(act.getByText("approved", { exact: true })).toBeVisible();

  await page.goto(`/#/p/${ids().case}/reports`);
  const rep = page.locator("section.card", { hasText: "Situation report 1" });
  await expect(rep.getByRole("heading", { name: "Situation report 1: Phished mailbox" })).toBeVisible();
  await rep.getByRole("button", { name: "Edit" }).click();
  await rep.getByRole("textbox").fill(sitrep() + "\nReviewed by the lead.\n");
  await rep.getByRole("button", { name: "Save" }).click();
  await expect(rep.getByText("edited by you")).toBeVisible();
  await rep.getByRole("button", { name: "Issue", exact: true }).click();
  await expect(rep.getByText("issued", { exact: true })).toBeVisible();
  expect(sitrep()).toContain("Reviewed by the lead.");
});

test("a report is copied, downloaded or printed alone", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(`/#/p/${ids().case}/reports`);
  const card = page.locator("section.card", { hasText: "Situation report 1" }).first();
  await card.getByRole("button", { name: "Situation report 1", exact: true }).click();
  const text = sitrep();
  await card.getByRole("button", { name: "Copy Markdown" }).click();
  await expect(card.getByText("Copied.")).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(text);
  const [dl] = await Promise.all([page.waitForEvent("download"), card.getByRole("button", { name: "Download .md" }).click()]);
  expect(dl.suggestedFilename()).toBe("sitrep-001.md");
  expect(readFileSync(await dl.path(), "utf8")).toBe(text);
  await page.evaluate(() => { window.print = () => { window.printed = document.getElementById("print-root").textContent; }; });
  await card.getByRole("button", { name: "Print / PDF" }).click();
  const printed = await page.evaluate(() => window.printed);
  expect(printed).toContain("Situation report 1");
  expect(printed).not.toContain("Case narrative");   // the report alone, not the page
});
