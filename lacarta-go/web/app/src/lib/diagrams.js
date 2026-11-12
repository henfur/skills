import { sanitizeSVG } from "./format.js";

// ```mermaid blocks in rendered markdown become SVG. The library loads only once a page has a
// diagram; renders are cached by theme and source, so a refresh re-inserts instead of redrawing.
const cache = new Map();
let loading = null, configured = "", seq = 0;

export const themeName = () => document.documentElement.dataset.theme
  || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");

function load() {
  if (!loading) loading = import("mermaid").then(m => m.default).catch(e => { loading = null; throw e; });
  return loading;
}

function configure(m) {
  const name = themeName();
  if (configured === name) return;
  const v = n => getComputedStyle(document.documentElement).getPropertyValue(`--${n}`).trim();
  m.initialize({
    startOnLoad: false,
    securityLevel: "strict",       // no scripts or click handlers in labels
    theme: "base",
    darkMode: name === "dark",
    fontFamily: "Manrope, ui-sans-serif, system-ui, sans-serif",
    flowchart: { htmlLabels: false, curve: "basis" },  // plain SVG text: no foreignObject HTML
    themeVariables: {
      fontFamily: "Manrope, ui-sans-serif, system-ui, sans-serif", fontSize: "14px",
      background: v("surface"), textColor: v("text"),
      primaryColor: v("brand-subtle"), primaryTextColor: v("text"), primaryBorderColor: v("brand-border"),
      secondaryColor: v("info-subtle"), secondaryTextColor: v("text"), secondaryBorderColor: v("info-border"),
      tertiaryColor: v("surface-sunken"), tertiaryTextColor: v("text"), tertiaryBorderColor: v("border"),
      lineColor: v("border-strong"), edgeLabelBackground: v("surface-raised"),
      clusterBkg: v("surface-sunken"), clusterBorder: v("border"), titleColor: v("text"),
      noteBkgColor: v("accent-subtle"), noteTextColor: v("text"), noteBorderColor: v("accent-border"),
      actorBkg: v("brand-subtle"), actorBorder: v("brand-border"), actorTextColor: v("text"),
      signalColor: v("text-secondary"), signalTextColor: v("text"), labelBoxBkgColor: v("surface-raised"),
      errorBkgColor: v("danger-subtle"), errorTextColor: v("danger"),
    },
  });
  configured = name;
}

const escapeHTML = s => String(s).replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const source = src => `<details class="diagram-src"><summary class="small">Source</summary><pre><code>${escapeHTML(src)}</code></pre></details>`;

export async function renderDiagrams(root) {
  const blocks = [...root.querySelectorAll("pre > code.language-mermaid")];
  if (!blocks.length) return;
  const figs = blocks.map(code => {
    const src = code.textContent, fig = document.createElement("figure");
    fig.className = "diagram";
    const hit = cache.get(themeName() + "\n" + src);
    fig.innerHTML = hit || `<div class="small muted">Drawing diagram…</div>`;
    code.parentElement.replaceWith(fig);
    return hit ? null : [fig, src];
  }).filter(Boolean);
  if (!figs.length) return;
  let m;
  try { m = await load(); } catch (e) {
    figs.forEach(([fig, src]) => { fig.innerHTML = `<div class="err">The diagram library could not be loaded.</div>${source(src)}`; });
    return;
  }
  configure(m);
  const theme = themeName();
  for (const [fig, src] of figs) {
    let html;
    try {
      const { svg } = await m.render(`diagram-${++seq}`, src);
      // Mermaid sanitises labels itself; sanitise the SVG again before it touches the page.
      html = sanitizeSVG(svg) + source(src);
      cache.set(theme + "\n" + src, html);
    } catch (e) {
      const msg = String((e && (e.str || e.message)) || e).split("\n")[0];
      html = `<div class="err">This diagram has a syntax error: ${escapeHTML(msg)}</div>${source(src)}`;
      // mermaid can leave its scratch nodes in the body when a diagram fails
      [`ddiagram-${seq}`, `diagram-${seq}`].forEach(id => document.getElementById(id)?.remove());
    }
    fig.innerHTML = html;
  }
}
