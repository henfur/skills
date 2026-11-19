// Getting a document out of the console: the clipboard, a .md file, or the printer (Save as PDF).

export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Outside a secure context (the console on a LAN address), the clipboard API is missing.
    const ta = Object.assign(document.createElement("textarea"), { value: text });
    ta.style.cssText = "position:fixed;top:0;left:0;opacity:0";
    document.body.append(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand("copy"); } catch { ok = false; }
    ta.remove();
    return ok;
  }
}

export function downloadText(text, filename, type = "text/markdown;charset=utf-8") {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = Object.assign(document.createElement("a"), { href: url, download: filename });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Prints a copy of `el` alone, in the light theme (see #print-root in app.css); the browser's
// dialog saves it as PDF.
export function printElement(el, title = "") {
  const root = document.documentElement;
  const theme = root.dataset.theme, docTitle = document.title;
  const box = Object.assign(document.createElement("div"), { id: "print-root" });
  box.append(el.cloneNode(true));
  document.body.append(box);
  root.dataset.printing = "";
  root.dataset.theme = "light";
  if (title) document.title = title; // the default file name of "Save as PDF"
  const done = () => {
    box.remove();
    delete root.dataset.printing;
    if (theme === undefined) delete root.dataset.theme; else root.dataset.theme = theme;
    document.title = docTitle;
    window.removeEventListener("afterprint", done);
  };
  window.addEventListener("afterprint", done);
  window.print();
}

export const slug = s => String(s || "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "document";

// A security audit finding as Markdown: what the Findings view shows, with the auditor's review.
export function findingMarkdown(f, level = 1) {
  const h = n => "#".repeat(level + n) + " ";
  const sev = f.severity ? f.severity[0].toUpperCase() + f.severity.slice(1) : "";
  const out = [`${h(0)}${f.title}`, "", [sev && `**Severity:** ${sev}`, `**Verdict:** ${String(f.final || f.verdict || "").replace(/_/g, " ")}`,
    `**ID:** \`${f.fingerprint}\``].filter(Boolean).join(" · "), ""];
  const add = (title, body) => { if (body) out.push(`${h(1)}${title}`, "", body, ""); };
  const list = (xs, fn, ordered) => xs?.length ? xs.map((x, i) => `${ordered ? `${i + 1}.` : "-"} ${fn(x)}`).join("\n") : "";
  if (f.review?.action === "severity") out.push(`Agents' severity: ${f.review.from}; the auditor's: ${f.review.severity}.`, "");
  add("Description", f.description);
  add("Root cause", f.root_cause || f.claimed_root_cause);
  add("Why the verifier rejected it", f.reason);
  add("Source trace", list(f.trace, t => `\`${t.file}:${t.line}\`${t.kind ? ` (${t.kind})` : ""} ${t.description}`, true));
  add("Evidence", list(f.evidence, e => `\`${e.file}:${e.line}\` ${e.description}`));
  add("Conditions", list(f.conditions, c => `${c.kind.replace(/_/g, " ")}: ${c.description}`));
  if (f.execution) add("Reproduction", [`Attacker: ${f.execution.attacker_perspective}`, "",
    list(f.execution.instructions, s => s, true), "", `Observed: ${f.execution.observed_result}`].join("\n"));
  if (f.remediation) add("Fix", [f.remediation.strategy, ...(f.remediation.code_changes || [])
    .map(c => `\n\`${c.file_name}\`\n\n\`\`\`\n${c.fixed_code}\n\`\`\``)].join("\n"));
  add("Unresolved", list(f.blockers, b => b));
  if (f.validation_plan) add("Validation plan", `- Locally: ${f.validation_plan.local}\n- In a test deployment: ${f.validation_plan.deployment}`);
  if (f.confidence) add("Agents' confidence", `${f.confidence.score}: ${f.confidence.reason}`);
  if (f.review?.reason) add("Auditor's review", f.review.reason);
  return out.join("\n").trimEnd() + "\n";
}

export const findingsMarkdown = (list, title) =>
  `# ${title}\n\n` + (list.length ? list.map(f => findingMarkdown(f, 2)).join("\n") : "None.\n");
