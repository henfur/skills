// The token the daemon put in the page: every POST and the event stream carry it.
export const TOKEN = document.querySelector('meta[name="lacarta-token"]')?.content ?? "";

const headers = { "X-Lacarta-Token": TOKEN };

export async function getText(url) {
  const r = await fetch(url, { cache: "no-store", headers });
  return { ok: r.ok, text: await r.text() };
}

// post returns { ok, json }; a network failure becomes { ok: false, json: { error } }.
export async function post(url, body) {
  try {
    const r = await fetch(url, {
      method: "POST", headers: { ...headers, "Content-Type": "application/json" }, body: JSON.stringify(body),
    });
    let json = {};
    try { json = await r.json(); } catch (e) {}
    return { ok: r.ok, json };
  } catch (e) {
    return { ok: false, json: { error: "The console server is not reachable. Check that `lacarta serve` is running, then try again." } };
  }
}

export const projectURL = (pid, action) => `/api/p/${encodeURIComponent(pid)}/${action}`;
