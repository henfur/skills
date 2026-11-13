// Run files an audit's agents would write, built with the skill's own helpers so its validators
// accept them. Usage: node audit-fixture.cjs <validate-coverage-ledger.cjs> <run dir> <plan.json|coverage-ledger.json>
process.argv.splice(1, 1);
const v = require(process.argv[1]);
const fs = require("fs"), path = require("path");
const out = process.argv[2];
const refs = { surface: "src/api.js#POST /orders/:id", boundary: "src/authz.js#requireOwner",
  subsystem: "api", attack_class: "ATTACK-CLASSES.md#Access control" };
const unit = (status, agent, fps) => ({ coverage_id: v.canonicalCoverageId(refs), canonical_refs: refs,
  surface: "Update order", boundary: "Order ownership", subsystem: "api", attack_class: "Access control",
  starting_paths: ["src/api.js"], ordinary_attack_class_block: "ATTACK-CLASSES.md#Access control",
  selected_companion_blocks: [], excluded_blocks: [], prior_status: "none", attempts: [], wave: 1, status,
  agent_id: agent, reviewed_paths: agent ? ["src/api.js"] : [], local_checks: agent ? [{ agent_id: agent,
  reviewed_paths: ["src/api.js"], invariant: "Only the owner updates an order.", method: "source",
  result: "The handler updates without the owner check.", artifact: null }] : [], result_fingerprints: fps, unresolved: [] });
fs.writeFileSync(path.join(out, process.argv[3]), JSON.stringify([process.argv[3] === "plan.json"
  ? unit("planned", null, []) : unit("candidate", "hunter-1", ["api-order-owner"])], null, 2));
const src = (kind, line) => ({ kind, file: "src/api.js", line, scope: "updateOrder", description: "The order id reaches the update." });
const sorted = xs => xs.sort((a, b) => a.fingerprint < b.fingerprint ? -1 : 1);
fs.writeFileSync(path.join(out, "findings.json"), JSON.stringify(sorted([
  { verdict: "confirmed", fingerprint: "api-order-owner", title: "Any user can update any order",
    description: "The update route changes an order without checking that it belongs to the caller.",
    root_cause: "updateOrder skips requireOwner.", intended_behavior: "Only the order's owner can change it.",
    trace: [src("entrypoint", 3), src("sink", 9)], evidence: [{ file: "src/api.js", line: 9, description: "No owner check." }],
    conditions: [], execution: { attacker_perspective: "A user with their own account.", payloads: ["Another user's order id."],
      instructions: ["Call the update route with that id."], observed_result: "The other user's order changes." },
    remediation: { strategy: "Call requireOwner before the update." },
    severity: { likelihood: { score: "high", reason: "Any user can reach it." }, impact: { score: "high", reason: "Orders of every user." },
      overall_severity: "high" }, confidence: { score: "high", reason: "Reproduced in a fixture." } },
  { verdict: "needs_validation", fingerprint: "api-size", title: "Unbounded body size", description: "Bodies may be unbounded.",
    claimed_root_cause: "No limit in the parser setup.", trace: [src("entrypoint", 1)], evidence: [{ file: "src/api.js", line: 1, description: "Parser setup." }],
    blockers: ["The proxy's limit isn't in the repository."], validation_plan: { local: "Send a large body to a local server.",
      deployment: "Check the proxy configuration." } },
  { verdict: "rejected", fingerprint: "api-auth", title: "Login bypass", description: "Claimed bypass.",
    claimed_root_cause: "Claimed missing check.", trace: [src("sink", 2)], evidence: [{ file: "src/api.js", line: 2, description: "Check present." }],
    reason: "The middleware checks every route." }]), null, 2));
