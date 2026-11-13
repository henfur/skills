import { marked } from "marked";
import DOMPurify from "dompurify";

export const SPEC_ORDER = ["functional", "architecture", "stack", "security"];
export const STAGES = [["specification", "Specification"], ["implementation", "Implementation"], ["final-review", "Final review"], ["done", "Done"]];
export const SEV_ORDER = ["critical", "high", "medium", "low", "info"];
// Security audits: their stages, and the skill's severity scale.
export const AUDIT_STAGES = [["scope", "Scope"], ["recon", "Recon"], ["coverage-plan", "Coverage plan"], ["hunt", "Hunting & validation"],
  ["review", "Your review"], ["report", "Report"], ["done", "Done"]];
export const AUDIT_SEV = ["critical", "high", "medium", "low", "informational"];
export const UNIT_TONE = { planned: "", in_progress: "info", covered: "success", candidate: "warning", blocked: "danger",
  deferred: "accent", out_of_scope: "" };
export const UNRESOLVED = new Set(["open", "fixing", "redesign"]);

export const label = s => String(s ?? "").replace(/-/g, " ").replace(/^./, c => c.toUpperCase());

// Everything agents write is markdown; it only reaches the page through DOMPurify.
export const md = s => DOMPurify.sanitize(marked.parse(String(s ?? "")));
export const mdInline = s => DOMPurify.sanitize(marked.parseInline(String(s ?? "")));
export const sanitizeSVG = svg => DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true }, ADD_TAGS: ["style"] });

export function agoText(iso, now = Date.now()) {
  if (!iso) return "";
  const s = Math.max(0, (now - Date.parse(iso)) / 1000);
  return s < 60 ? "just now" : s < 3600 ? `${Math.floor(s / 60)} min ago` : s < 86400 ? `${Math.floor(s / 3600)} h ago` : `${Math.floor(s / 86400)} d ago`;
}

export const TONE = {
  approved: "success", done: "success", pass: "success", fixed: "success", answered: "success",
  "in-review": "info", "in-progress": "info", "awaiting-approval": "brand", open: "warning",
  "changes-requested": "danger", fail: "danger", fixing: "warning", redesign: "warning",
  accepted: "accent", "wont-fix": "", draft: "", planned: "", stopped: "danger", changed: "warning",
  passed: "success", failed: "danger", stale: "warning", none: "",
  specification: "info", implementation: "brand", "final-review": "accent",
};
export const sevTone = s => s === "critical" ? "critical" : s === "high" ? "danger" : s === "medium" ? "warning" : s === "low" ? "info" : "";

export const AGENT_STATUS = {
  working: ["in-progress", "Working"], waiting: ["awaiting-approval", "Waiting for you"], blocked: ["fail", "Blocked"],
  listening: ["accepted", "Listening to the console"], terminal: ["open", "Needs you in the terminal"],
  idle: ["draft", "Idle"], done: ["done", "Done"], failed: ["fail", "Failed"],
};
export const agentStatus = s => AGENT_STATUS[s] || ["draft", label(s)];

export const phaseName = p => p === "final" ? "Final review" : p === "spec" ? "Specification" : p == null ? "" : `Phase ${p}`;

export const SPEC_REVIEW_TEXT = {
  none: ["Not run yet", "Once all four specs and the plan are written, a fresh agent with no memory reviews them for gaps and incoherences. Approvals open after it passes."],
  failed: ["Failed", "The last independent review found problems. The agent fixes them, then a new reviewer checks again."],
  stale: ["Out of date", "The specs or the plan changed after the last review passed. A new independent review is needed before approval."],
  passed: ["Passed on the current text", "An independent agent found the specification complete and coherent. Any further edit requires a new review."],
};
export const SPEC_STEP_WORD = { none: "Writing", failed: "Fixing review findings", stale: "Needs a new review", passed: "Awaiting your approval" };

// Agents are shown by what they do, every one of them, finished or not.
export const AGENT_GROUPS = [
  ["Agent", ["agent"]],
  ["Orchestrator", ["orchestrator"]],
  ["Developers", ["implementation"]],
  ["Reviewers", ["spec_review", "functional_review", "security_review"]],
  ["Final reviewers", ["final_functional_review", "final_security_review"]],
];
// Within a group: whoever needs you first, then the busy ones, then the finished ones.
export const STATUS_ORDER = ["waiting", "terminal", "blocked", "listening", "working", "idle", "failed", "done"];
export const STATUS_DOT = { waiting: "brand", terminal: "warning", blocked: "danger", listening: "accent", working: "info", failed: "danger" };

const AVATAR_TONES = ["brand", "info", "success", "accent", "warning"];
export const avatarTone = id => AVATAR_TONES[Math.abs([...String(id)].reduce((h, c) => h * 31 + c.charCodeAt(0), 7)) % AVATAR_TONES.length];
export const initials = name => String(name || "?").split(/\s+/).map(w => w[0]).join("").slice(0, 2).toUpperCase();
// `avatar` is reserved for custom pictures; only inline images are rendered, never remote URLs.
export const avatarPicture = a => typeof a?.avatar === "string" && /^data:image\/(png|jpeg|gif|webp);base64,/.test(a.avatar) ? a.avatar : "";
