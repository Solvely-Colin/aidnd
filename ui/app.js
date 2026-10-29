// aidnd player UI. No build step: plain modules, template strings and one
// delegated click handler. Every call goes to /api/* on the Worker, which
// adds the player's id from the session cookie.

const $ = (s, el = document) => el.querySelector(s);
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const app = $("#app");

const svg = (d, w = 1.8) => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round">${d}</svg>`;
const ICON = {
  back: svg(`<path d="M15 18l-6-6 6-6"/>`, 2),
  send: svg(`<path d="M5 12h14M13 6l6 6-6 6"/>`, 2.2),
  story: svg(`<path d="M4 5h16M4 10h16M4 15h10M4 20h7"/>`),
  scene: svg(`<path d="M12 21s-7-6.2-7-11.5a7 7 0 0 1 14 0C19 14.8 12 21 12 21z"/><circle cx="12" cy="9.5" r="2.5"/>`),
  you: svg(`<circle cx="12" cy="8" r="4"/><path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6"/>`),
  book: svg(`<path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z"/><path d="M4 21V5M9 7h6"/>`),
  map: svg(`<path d="M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2z"/><path d="M9 4v14M15 6v14"/>`),
  close: svg(`<path d="M6 6l12 12M18 6L6 18"/>`, 2),
  plus: svg(`<path d="M12 5v14M5 12h14"/>`, 2),
  look: svg(`<circle cx="11" cy="11" r="6"/><path d="M20 20l-4.5-4.5"/>`),
  talk: svg(`<path d="M4 5h16v11H9l-5 4z"/>`),
  go: svg(`<path d="M5 12h12M13 6l6 6-6 6"/>`),
  fight: svg(`<path d="M14.5 4H20v5.5L9 20.5 3.5 15z"/><path d="M5 13l6 6M3 21l3-3"/>`),
  use: svg(`<path d="M6 8h12l-1 12H7z"/><path d="M9 8V6a3 3 0 0 1 6 0v2"/>`),
  end: svg(`<path d="M6 6h12v12H6z"/>`),
  heart: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-8-5.2-8-11a4.5 4.5 0 0 1 8-2.8A4.5 4.5 0 0 1 20 10c0 5.8-8 11-8 11z"/></svg>`,
  logo: `<svg class="logo" viewBox="0 0 32 32" aria-hidden="true"><path d="M16 2 29 9.5v13L16 30 3 22.5v-13z" fill="var(--accent)"/><path d="M16 8 23 20H9z" fill="var(--bg)"/></svg>`,
};

const PREMISES = [
  "A 1930s Lovecraftian detective story in a fog-bound harbor town",
  "A heist in a floating city of clockwork and canals",
  "Survivors of a shipwreck on an island that remembers them",
  "A frontier town where the dead keep coming back to vote",
];
const CONCEPTS = [
  "a tired private eye with a flask and a debt to the wrong people",
  "a sharp-eyed tracker who reads a room like a trail",
  "a disgraced professor of folklore who knows too much",
  "a dock worker with fast fists and a soft heart",
];

const S = {
  me: null,          // {player_id, name}
  meta: null,
  route: "",
  view: null,        // campaign view
  events: [],
  lastId: 0,
  es: null,
  draft: null,       // character being edited
  progress: null,    // how far the action in flight has got (unsaved stream event)
  step: 0,           // character builder step
  composing: false,  // new story form open on home
  sending: false,
  tab: "story",      // phone: story | scene | you | journal | map
  panel: "scene",    // wide side column: scene | you | journal | map
  tray: "",          // open action group in the dock
  journal: null,
  map: null,
  hint: {},
  composer: "",
};

const wide = () => innerWidth >= 1000;

// ---- api

class HttpError extends Error {
  constructor(status, data) {
    super((data && (data.problems ? data.problems.join(" ") : data.error)) || `Request failed (${status})`);
    this.status = status;
  }
}

async function api(method, path, body) {
  const r = await fetch("/api" + path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
  });
  let data = null;
  try { data = await r.json(); } catch {}
  if (r.status === 401 && data?.error === "Sign in first.") { S.me = null; renderSignin(); }
  if (!r.ok) throw new HttpError(r.status, data);
  return data;
}

let toastTimer;
function toast(msg) {
  let el = $(".toast");
  if (!el) { el = document.createElement("div"); el.className = "toast"; document.body.appendChild(el); }
  el.textContent = msg;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.remove(), 3800);
}

// ---- cover art: a small landscape drawn from the story's id, so every
// story has its own picture without any image generation.

function hash(s) { let h = 2166136261; for (const c of String(s)) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; }
const COVERS = [
  ["#16201b", "#24352c", "#33503f", "#c8f53c"],
  ["#1f1716", "#352321", "#55322c", "#ff7a59"],
  ["#141b26", "#1f2b3d", "#2d4260", "#7cc4ff"],
  ["#1d1a12", "#322c1b", "#4f4426", "#f5c542"],
  ["#1a1a1a", "#2a2a2a", "#404040", "#f2f2f0"],
];
function cover(seed, cls = "") {
  let h = hash(seed);
  const rnd = () => { h = Math.imul(h ^ (h >>> 15), 2246822507) >>> 0; return (h % 1000) / 1000; };
  const [sky, far, near, glow] = COVERS[hash(seed) % COVERS.length];
  const ridge = (base, amp, n) => {
    let d = `M0 ${base}`;
    for (let i = 1; i <= n; i++) d += ` L${(i * 400) / n} ${base - rnd() * amp}`;
    return d + " L400 225 L0 225Z";
  };
  const mx = 60 + rnd() * 280, my = 40 + rnd() * 50, mr = 18 + rnd() * 22;
  const stars = Array.from({ length: 14 }, () => `<circle cx="${(rnd() * 400).toFixed(0)}" cy="${(rnd() * 110).toFixed(0)}" r="${(0.6 + rnd()).toFixed(1)}" fill="${glow}" opacity="${(0.3 + rnd() * 0.5).toFixed(2)}"/>`).join("");
  return `<svg class="cover ${cls}" viewBox="0 0 400 225" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
    <rect width="400" height="225" fill="${sky}"/>${stars}
    <circle cx="${mx}" cy="${my}" r="${mr}" fill="${glow}"/>
    <path d="${ridge(150, 60, 7)}" fill="${far}"/>
    <path d="${ridge(185, 45, 11)}" fill="${near}"/>
    <path d="M0 205 H400 V225 H0Z" fill="${sky}" opacity=".9"/>
  </svg>`;
}

// ---- generated art. The Worker paints a cover for each story and a
// portrait for each character the first time one is asked for. Until the
// picture exists it answers 202, the image fails to load, and the drawn
// cover or the initial underneath stays in view while we ask again.

const ART = new Map(); // src -> "ok" | "gone" | tries so far
const ART_TRIES = 15, ART_WAIT = 6000;
const coverArt = id => artImg(`/img/${id}/cover`);
// Only the place the party stands in can be painted (the Worker reads it
// from the campaign view), so other places keep their drawn art.
const placeArt = (id, cls) => `<div class="art-box ${cls}">${cover(id || "place")}${id && id === S.view?.location?.id ? artImg(`/img/${S.route}/place/${id}`) : ""}</div>`;
const portrait = c => c?.id && S.route ? artImg(`/img/${S.route}/char/${c.id}`) : "";
function artImg(src) {
  const st = ART.get(src);
  if (st === "gone") return "";
  return `<img class="gen ${st === "ok" ? "on" : ""}" src="${src}" alt="" data-art="${src}">`;
}
const artEls = src => document.querySelectorAll(`img[data-art="${CSS.escape(src)}"]`);
document.addEventListener("load", e => {
  const src = e.target?.dataset?.art;
  if (!src) return;
  ART.set(src, "ok");
  artEls(src).forEach(img => { if (img.naturalWidth) img.classList.add("on"); });
}, true);
document.addEventListener("error", e => {
  const src = e.target?.dataset?.art, st = src && ART.get(src);
  if (!src || st === "ok" || st === "gone" || st === "wait") return;
  const tries = (typeof st === "number" ? st : 0) + 1;
  if (tries >= ART_TRIES) { ART.set(src, "gone"); artEls(src).forEach(img => img.remove()); return; }
  ART.set(src, "wait");
  setTimeout(() => { ART.set(src, tries); artEls(src).forEach(img => { img.src = `${src}?try=${tries}`; }); }, ART_WAIT);
}, true);

// ---- routing

function go(hash) { if (location.hash !== hash) location.hash = hash; else route(); }
window.addEventListener("hashchange", route);

async function route() {
  if (!S.me) return renderSignin();
  const m = location.hash.match(/^#\/c\/([\w-]+)/);
  const next = m ? m[1] : "";
  if (next !== S.route) leaveCampaign();
  S.route = next;
  if (!next) return renderHome();
  await openCampaign(next);
}

function leaveCampaign() {
  S.es?.close();
  Object.assign(S, { es: null, view: null, events: [], lastId: 0, progress: null, draft: null, step: 0, journal: null, map: null, tab: "story", tray: "", composer: "", hint: {} });
}

// ---- sign in

function renderSignin(err = "") {
  app.innerHTML = `
  <div class="signin">
    <div class="signin-art">${cover("aidnd-sign-in")}<div class="signin-tag">Tell a story. Roll the dice.<br>Let the game master keep score.</div></div>
    <div class="signin-side">
      <form id="signin" class="signin-form">
        <div class="signin-head">${ICON.logo}<h1>aidnd</h1><p>An AI game master for you and your friends.</p></div>
        <label class="field"><span>Your name</span><input class="input" name="name" autocomplete="nickname" maxlength="40" required value="${esc(localName())}"></label>
        <label class="field"><span>Play code</span><input class="input" name="code" type="password" autocomplete="current-password" required></label>
        <div class="error">${esc(err)}</div>
        <button class="btn primary block">Sign in</button>
      </form>
    </div>
  </div>`;
  $("#signin").onsubmit = async e => {
    e.preventDefault();
    const f = new FormData(e.target);
    const btn = $("button", e.target);
    btn.disabled = true;
    try {
      const r = await fetch("/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: f.get("name"), code: f.get("code") }) });
      const d = await r.json();
      if (!r.ok) throw new Error(d.error || "Could not sign in.");
      try { localStorage.setItem("aidnd-name", d.name); } catch {}
      S.me = d;
      route();
    } catch (er) { renderSignin(er.message); }
  };
}
function localName() { try { return localStorage.getItem("aidnd-name") || ""; } catch { return ""; } }

// ---- home

function topbar(inner) { return `<header class="topbar">${inner}</header>`; }

async function renderHome() {
  app.innerHTML = `
  <div class="page">
    ${topbar(`<span class="brand">${ICON.logo}aidnd</span><span class="grow"></span>
      <button class="me-btn" data-act="signout" title="Sign out">${esc(S.me.name)}<span class="avatar sm">${esc(S.me.name[0] || "?")}</span></button>`)}
    <main class="wrap wide-wrap">
      <div class="hero"><h2>Your table</h2><p>Pick up a story, join a friend's, or start a new one.</p></div>
      ${S.composing ? newStoryHTML() : ""}
      <div id="lists"><div class="shelf">${`<div class="story-card skeleton"></div>`.repeat(2)}</div></div>
    </main>
  </div>`;
  if (S.composing) $("#premise")?.focus();
  try {
    const [mine, all] = await Promise.all([api("GET", "/me/campaigns"), api("GET", "/campaigns")]);
    if (S.route) return;
    const mineIds = new Set(mine.campaigns.map(c => c.id));
    const open = all.campaigns.filter(c => c.status === "lobby" && !mineIds.has(c.id) && c.players < c.max_players);
    const newTile = S.composing ? "" : `<button class="story-card new-tile" data-act="compose"><span class="plus">${ICON.plus}</span><b>New story</b><span>The game master writes the world in about a minute.</span></button>`;
    $("#lists").innerHTML = `
      <section class="section"><div class="section-head"><h2>Your stories</h2><span class="count">${mine.campaigns.length}</span></div>
        <div class="shelf">${newTile}${mine.campaigns.map(storyCard).join("")}</div></section>
      ${open.length ? `<section class="section"><div class="section-head"><h2>Open tables</h2><span class="count">${open.length}</span></div><div class="shelf">${open.map(storyCard).join("")}</div></section>` : ""}`;
  } catch (e) { $("#lists").innerHTML = `<div class="error">${esc(e.message)}</div>`; }
}

function storyCard(c) {
  return `<button class="story-card" data-open="${esc(c.id)}">
    <div class="art">${cover(c.id)}${coverArt(c.id)}${statusPill(c.status)}</div>
    <div class="body"><h3>${esc(c.title)}</h3>${c.pitch ? `<p>${esc(c.pitch)}</p>` : ""}
      <div class="meta"><span>${c.players}/${c.max_players} players</span><span>${ago(c.updated_at)}</span></div></div>
  </button>`;
}

function newStoryHTML() {
  return `<section class="compose" id="new">
    <div class="compose-head"><h3>New story</h3><button class="icon-btn" data-act="compose-close" aria-label="Close">${ICON.close}</button></div>
    <textarea class="textarea" id="premise" rows="3" placeholder="What is this story about? A place, a mood, a mystery…"></textarea>
    <div class="chips">${PREMISES.map(p => `<button class="chip" data-premise="${esc(p)}">${esc(p)}</button>`).join("")}</div>
    <div class="compose-foot"><span class="muted">Takes about a minute.</span><button class="btn primary" data-act="create">Create story</button></div>
  </section>`;
}

function statusPill(s) {
  return { lobby: `<span class="pill">Gathering</span>`, active: `<span class="pill accent">In play</span>`, completed: `<span class="pill">Finished</span>`, failed: `<span class="pill bad">Lost</span>` }[s] || "";
}
function ago(ts) {
  if (!ts) return "";
  const s = Math.max(1, Date.now() / 1000 - ts);
  if (s < 3600) return `${Math.round(s / 60) || 1} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}

async function createCampaign() {
  const premise = $("#premise").value.trim();
  if (!premise) { $("#premise").focus(); return toast("Write a line about the story first."); }
  const started = Date.now();
  $("#new").innerHTML = `<div class="writing"><span class="dots"><i></i><i></i><i></i></span><h3>Writing your world</h3><p class="muted"><span id="elapsed">0</span>s · places, people, secrets</p></div>`;
  const t = setInterval(() => { const el = $("#elapsed"); if (el) el.textContent = Math.round((Date.now() - started) / 1000); }, 1000);
  try {
    const c = await api("POST", "/campaigns", { premise });
    await api("POST", `/campaigns/${c.id}/players`, { name: S.me.name });
    S.composing = false;
    go(`#/c/${c.id}`);
  } catch (e) { toast(e.message); renderHome(); }
  finally { clearInterval(t); }
}

// ---- campaign

async function openCampaign(id) {
  if (!S.view) app.innerHTML = `<div class="center-msg"><span class="spinner muted"></span></div>`;
  try {
    S.view = await api("GET", `/campaigns/${id}`);
  } catch (e) {
    app.innerHTML = `<div class="center-msg"><div class="card stack"><div class="error">${esc(e.message)}</div><button class="btn" onclick="location.hash='#/'">Back home</button></div></div>`;
    return;
  }
  if (!S.meta) api("GET", "/meta").then(m => { S.meta = m; if (S.view?.status === "lobby") render(); }).catch(() => {});
  connect(id);
  render();
}

function connect(id) {
  if (S.es) return;
  const es = new EventSource(`/api/campaigns/${id}/events`);
  const types = ["campaign_created", "player_joined", "character_created", "player_action", "action_resolved", "narration", "enemy_turns", "quest_updated", "location_changed", "turn_started", "turn_budget", "campaign_ended"];
  types.forEach(t => es.addEventListener(t, ev => onEvent(JSON.parse(ev.data))));
  // Progress events are not saved and have no id: they say the action is
  // being read, then carry its dice while the narration is written.
  es.addEventListener("progress", ev => { S.progress = JSON.parse(ev.data).data; renderFeed(); });
  S.es = es;
}

let refreshTimer, feedTimer;
function onEvent(e) {
  if (e.id <= S.lastId) return;
  S.lastId = e.id;
  S.events.push(e);
  if (e.type !== "character_created" && e.type !== "player_joined") S.progress = null;
  clearTimeout(feedTimer);
  feedTimer = setTimeout(renderFeed, 30);
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refreshView, 250);
}

async function refreshView() {
  if (!S.route) return;
  const before = S.view?.status;
  try { S.view = await api("GET", `/campaigns/${S.route}`); } catch { return; }
  S.journal = null; S.map = null;
  if (before !== S.view.status) return render();
  if (S.view.status === "lobby") return renderSetup(true);
  renderChrome();
}

function render() {
  if (!S.view) return;
  if (S.view.status === "lobby") return renderSetup();
  renderPlay();
}

function mine() {
  const v = S.view;
  return v?.you?.character_id ? v.party.find(c => c.id === v.you.character_id) : null;
}

// ---- setup (lobby)

function renderSetup(soft) {
  const v = S.view, me = mine();
  if (soft && S.draft) {
    // Keep the builder as it is; only the party strip changes.
    const p = $("#party"); if (p) p.innerHTML = partyStrip();
    return;
  }
  // The builder gets the whole screen; the party shrinks to a strip of faces.
  if (S.draft && v.you?.joined) {
    app.innerHTML = `
    <div class="page builder-page">
      <header class="topbar"><button class="icon-btn" data-act="cancel-draft" aria-label="Stop making a character">${ICON.close}</button>
        <div class="titles"><span class="eyebrow">${esc(v.title)}</span><h1>Your character</h1></div>
        <div class="party-strip" id="party">${partyStrip()}</div></header>
      <main class="wrap">${builderHTML()}</main>
    </div>`;
    wireBuilder();
    return;
  }
  const allReady = v.players.length > 0 && v.players.every(p => p.character_id);
  app.innerHTML = `
  <div class="page setup">
    <div class="setup-hero">${cover(S.route, "hero-art")}${coverArt(S.route)}
      <button class="icon-btn on-art" data-act="home" aria-label="Back">${ICON.back}</button>
      <div class="setup-title"><span class="pill">Gathering the party</span><h1>${esc(v.title)}</h1></div>
    </div>
    <main class="wrap">
      <p class="pitch">${esc(v.pitch || v.premise)}</p>
      ${v.you?.joined ? (me ? readyHTML(me) : conceptHTML()) : ""}
      <section class="section">
        <div class="section-head"><h2>Party</h2><span class="count">${v.players.length}/${v.max_players}</span></div>
        <div class="party-row" id="party">${partyRow()}</div>
        ${!v.you?.joined ? `<button class="btn primary block" data-act="join">Join as ${esc(S.me.name)}</button>` : ""}
      </section>
    </main>
    ${v.you?.joined && !S.draft ? `<div class="start-bar"><div class="start-inner">
      <span class="muted">${allReady ? "Everyone is ready." : "Waiting for every player to make a character."}</span>
      <button class="btn primary" data-act="start" ${allReady ? "" : "disabled"}>Begin the story</button></div></div>` : ""}
  </div>`;
  wireBuilder();
}

function partyRow() {
  const v = S.view;
  const seats = Array.from({ length: v.max_players }, (_, i) => v.players[i]);
  return seats.map(p => {
    if (!p) return `<div class="seat empty"><div class="avatar lg">+</div><b>Open seat</b><span>Share the play code</span></div>`;
    const c = v.party.find(x => x.id === p.character_id);
    return `<div class="seat ${c ? "ready" : ""}"><div class="avatar lg">${esc((c?.name || p.name)[0] || "?")}${portrait(c)}</div>
      <b>${esc(c ? c.name : p.name)}</b><span>${c ? esc(c.class) : "Making a character…"}</span></div>`;
  }).join("");
}

function partyStrip() {
  const v = S.view;
  return v.players.map(p => {
    const c = v.party.find(x => x.id === p.character_id);
    return `<span class="avatar sm ${c ? "ready" : ""}" title="${esc(c ? c.name : p.name + " is making a character")}">${esc((c?.name || p.name)[0] || "?")}${portrait(c)}</span>`;
  }).join("");
}

function conceptHTML() {
  const pick = CONCEPTS[Math.floor(Math.random() * CONCEPTS.length)];
  return `<section class="section">
    <div class="section-head"><h2>Your character</h2></div>
    <div class="panel-card stack">
      <label class="field"><span>Describe them in a sentence</span>
        <textarea class="textarea" id="concept" rows="3" placeholder="e.g. ${esc(pick)}"></textarea></label>
      <span class="muted small">Age, trade, a flaw, a want. The game master drafts a sheet you can change.</span>
      <button class="btn primary" data-act="draft">Draft my character</button>
    </div>
  </section>`;
}

function readyHTML(c) {
  return `<section class="section">
    <div class="section-head"><h2>Your character</h2><button class="btn small" data-act="edit">Edit</button></div>
    <div class="panel-card char-card">
      <div class="avatar xl">${esc(c.name[0])}${portrait(c)}</div>
      <div class="grow"><h3>${esc(c.name)}</h3><div class="muted small">${esc(c.class)} · ${esc(c.pronouns || "they/them")} · ${esc(kitName(c.kit))}</div>
      ${c.backstory ? `<p class="muted">${esc(c.backstory)}</p>` : ""}</div>
    </div>
  </section>`;
}

// Kits are reskinned per campaign setting (a 1930s game gets pistols, not swords).
function kitList() { return S.meta?.settings?.find(s => s.key === S.view?.setting)?.kits || S.meta?.kits || []; }
function kitName(k) { return kitList().find(x => x.key === k)?.name || k; }
const ABIL = ["STR", "DEX", "CON", "INT", "WIS", "CHA"];
const ABIL_NAME = { STR: "Strength", DEX: "Dexterity", CON: "Constitution", INT: "Intelligence", WIS: "Wisdom", CHA: "Charisma" };
const mod = n => Math.floor((n - 10) / 2);
const signed = n => (n >= 0 ? "+" : "") + n;

// The builder walks through four steps; every step edits the same draft.
const STEPS = ["Who", "Kit", "Abilities", "Story"];

function builderHTML() {
  const d = S.draft, i = S.step;
  const body = [
    () => `<div class="grid2">
        <label class="field"><span>Name</span><input class="input" id="d-name" value="${esc(d.name)}" maxlength="40"></label>
        <label class="field"><span>Pronouns</span><input class="input" id="d-pron" value="${esc(d.pronouns || "they/them")}" maxlength="20"></label>
      </div>
      <label class="field"><span>Calling</span><input class="input" id="d-class" value="${esc(d.class)}" placeholder="Private eye, occultist, boxer…"></label>`,
    () => `<div class="kits">${kitList().map(k => `<button class="kit ${k.key === d.kit ? "on" : ""}" data-kit="${esc(k.key)}"><b>${esc(k.name)}</b><span>${esc(k.desc)}</span><span class="kit-items">${esc((k.items || []).map(it => it.name).join(" · "))}</span></button>`).join("")}</div>`,
    () => {
      const arr = S.meta?.standard_array || [15, 14, 13, 12, 10, 8];
      return `<p class="muted small">Pick a value for each, then star your two strengths.</p>
      <div class="abilities">${ABIL.map(a => {
        const v = d.scores?.[a] ?? 10, prof = (d.proficient || []).includes(a);
        return `<div class="ability ${prof ? "is-prof" : ""}" title="${ABIL_NAME[a]}">
          <button class="prof" data-prof="${a}" aria-label="Proficient in ${ABIL_NAME[a]}">${prof ? "★" : "☆"}</button>
          <div class="name">${ABIL_NAME[a]}</div>
          <select data-ab="${a}" aria-label="${ABIL_NAME[a]}">${arr.map(x => `<option ${x === v ? "selected" : ""}>${x}</option>`).join("")}</select>
          <div class="mod">${signed(mod(v))}</div></div>`;
      }).join("")}</div>`;
    },
    () => `<label class="field"><span>Backstory</span><textarea class="textarea" id="d-back" rows="6">${esc(d.backstory)}</textarea></label>`,
  ][i]();
  return `<section class="section builder" id="editor">
    <div class="steps">${STEPS.map((s, n) => `<button class="step ${n === i ? "on" : n < i ? "done" : ""}" data-step="${n}"><i></i><span>${s}</span></button>`).join("")}</div>
    <div class="panel-card stack">
      <h3>${["Who are they?", "Pick a kit", "Set abilities", "Their story"][i]}</h3>
      ${body}
      <div class="error" id="d-err"></div>
    </div>
    <div class="builder-foot">
      <button class="btn ghost" data-act="${i === 0 ? "cancel-draft" : "step-back"}">${i === 0 ? "Cancel" : "Back"}</button>
      <span class="muted small">${i + 1} of ${STEPS.length}</span>
      ${i < STEPS.length - 1 ? `<button class="btn primary" data-act="step-next">Next</button>` : `<button class="btn primary" data-act="save-char">Save character</button>`}
    </div>
  </section>`;
}

function readEditor() {
  const d = S.draft;
  if (!d) return d;
  const val = id => $(id)?.value;
  if ($("#d-name")) { d.name = val("#d-name"); d.pronouns = val("#d-pron"); d.class = val("#d-class"); }
  if ($("#d-back")) d.backstory = val("#d-back");
  return d;
}

function wireBuilder() {
  document.querySelectorAll("[data-ab]").forEach(sel => sel.onchange = () => {
    const d = readEditor(), a = sel.dataset.ab, nv = +sel.value, old = d.scores[a];
    // Keep the standard array a permutation: swap with whoever had the value.
    const other = ABIL.find(x => x !== a && d.scores[x] === nv);
    if (other) d.scores[other] = old;
    d.scores[a] = nv;
    renderSetup();
  });
}

// ---- play
//
// Phones get one view at a time with a bottom nav (story, scene, you,
// journal, map). Wide screens show the party rail, the story and a side
// column together.

function renderPlay() {
  app.innerHTML = `
  <div class="play" data-view="${S.tab}">
    <header class="hud" id="hud"></header>
    <div class="play-body">
      <aside class="rail" id="rail"></aside>
      <section class="story">
        <div class="scene-bar" id="scene-bar"></div>
        <div class="feed-scroll" id="scroll"><div class="feed" id="feed"></div></div>
        <footer class="dock" id="dock"></footer>
      </section>
      <aside class="side" id="side"></aside>
    </div>
    <nav class="bottom-nav" id="nav"></nav>
  </div>`;
  renderChrome();
  renderFeed(true);
}

function renderChrome() {
  if (!$("#hud")) return;
  renderHud(); renderRail(); renderSceneBar(); renderDock(); renderSide(); renderNav();
}

function renderHud() {
  const v = S.view, me = mine();
  $("#hud").innerHTML = `
    <button class="icon-btn" data-act="home" aria-label="Back">${ICON.back}</button>
    <div class="titles"><h1>${esc(v.title)}</h1><div class="sub">${v.scene ? `Day ${v.scene.day} · ${esc(v.scene.time)}` : ""}</div></div>
    <div class="party-mini">${v.party.map(c => `<button class="mini ${v.active?.character_id === c.id ? "acting" : ""} ${c.down ? "down" : ""}" data-tab="you" title="${esc(c.name)} ${c.hp}/${c.max_hp}">
      <span class="avatar sm">${esc(c.name[0])}${portrait(c)}</span>${ring(c.hp, c.max_hp)}</button>`).join("")}</div>
    ${me ? `<span class="hp ${me.hp <= me.max_hp / 3 ? "low" : ""}">${ICON.heart}${me.hp}<small>/${me.max_hp}</small></span>` : ""}`;
}

function ring(hp, max) {
  const p = Math.max(0, Math.min(1, hp / max)), c = 2 * Math.PI * 17;
  return `<svg class="ring ${p <= .34 ? "low" : ""}" viewBox="0 0 40 40"><circle cx="20" cy="20" r="17"/><circle class="fill" cx="20" cy="20" r="17" stroke-dasharray="${(c * p).toFixed(1)} ${c.toFixed(1)}"/></svg>`;
}

function renderRail() {
  const v = S.view, me = mine();
  $("#rail").innerHTML = `
    <p class="rail-title">Party</p>
    ${v.party.map(c => `<div class="member ${v.active?.character_id === c.id ? "acting" : ""} ${c.id === me?.id ? "me" : ""}">
      <div class="avatar">${esc(c.name[0])}${portrait(c)}</div>
      <div class="grow"><b>${esc(c.name)}${c.id === me?.id ? ` <span class="muted">(you)</span>` : ""}</b><span>${esc(c.class)} · lvl ${c.level ?? 1}</span>${hpbar(c.hp, c.max_hp)}</div>
      <span class="num">${c.hp}</span></div>`).join("")}
    ${v.encounter ? `<p class="rail-title">Enemies · round ${v.encounter.round}</p>${v.encounter.enemies.map(e => `<div class="member foe ${e.hp <= 0 ? "down" : ""}"><div class="avatar">${esc(e.display[0])}</div>
      <div class="grow"><b>${esc(e.display)}</b>${e.hp > 0 ? hpbar(e.hp, e.max_hp) : "<span>Defeated</span>"}</div><span class="num">${Math.max(0, e.hp)}</span></div>`).join("")}` : ""}`;
}

function renderSceneBar() {
  const v = S.view, l = v.location || {};
  $("#scene-bar").innerHTML = `
    <div class="where"><span class="label">${v.encounter ? `<b class="combat">Combat · round ${v.encounter.round}</b>` : "You are in"}</span><h2>${esc(l.name || "")}</h2></div>
    <button class="chip" data-tab="scene">${(l.npcs?.length || 0) + (l.items?.length || 0)} here · ${l.exits?.length || 0} ways out</button>`;
}

function renderNav() {
  const tabs = [["story", "Story", ICON.story], ["scene", "Scene", ICON.scene], ["you", "You", ICON.you], ["journal", "Journal", ICON.book], ["map", "Map", ICON.map]];
  $("#nav").innerHTML = tabs.map(([k, l, ic]) => `<button class="nav-item ${S.tab === k ? "on" : ""}" data-tab="${k}">${ic}<span>${l}</span></button>`).join("");
}

function renderSide() {
  const el = $("#side");
  if (!el) return;
  const key = wide() ? S.panel : (S.tab === "story" ? S.panel : S.tab);
  if (key === "journal" && journalBusy()) { J.stale = true; return; }
  const tabs = [["scene", "Scene"], ["you", "You"], ["journal", "Journal"], ["map", "Map"]];
  el.innerHTML = `<div class="side-tabs">${tabs.map(([k, l]) => `<button class="tab ${key === k ? "on" : ""}" data-panel="${k}">${l}</button>`).join("")}</div>
    <div class="side-body">${({ scene: sceneHTML, you: youHTML, journal: journalHTML, map: mapHTML })[key]()}</div>`;
}

// feed

function nearBottom() { const s = $("#scroll"); return !s || s.scrollHeight - s.scrollTop - s.clientHeight < 160; }
function toBottom() { const s = $("#scroll"); if (s) s.scrollTop = s.scrollHeight; }

function renderFeed(force) {
  const feed = $("#feed");
  if (!feed) { if (S.view?.status === "lobby") renderSetup(true); return; }
  const stick = force || nearBottom();
  const items = feedItems(currentEvents(S.events));
  const lastNarr = items.map(it => it.narr ? 1 : 0).lastIndexOf(1);
  feed.innerHTML = items.map((it, i) => it.kind === "turn" ? turnHTML(it, i === lastNarr) : eventHTML(it.e, i === lastNarr)).join("") + pendingHTML();
  if (stick) requestAnimationFrame(toBottom);
}

// currentEvents drops character sheets a player later replaced in the
// lobby, so each seat joins the party once.
function currentEvents(evs) {
  const last = {};
  evs.forEach((e, i) => { if (e.type === "character_created") last[e.data?.player_id] = i; });
  return evs.filter((e, i) => e.type !== "character_created" || last[e.data?.player_id] === i);
}

// feedItems groups a player's action, its dice, the narration and what
// changed into one turn. Turn markers are dropped: the dock says whose
// turn it is.
function feedItems(evs) {
  const out = [];
  let turn = null;
  for (const e of evs) {
    const d = e.data || {};
    if (e.type === "player_action") { turn = { kind: "turn", act: e, resolved: null, narr: null, quests: [] }; out.push(turn); continue; }
    if (turn && e.type === "action_resolved" && !turn.resolved) { turn.resolved = e; continue; }
    if (turn && e.type === "narration" && !turn.narr && !d.setback) { turn.narr = e; continue; }
    if (turn && e.type === "quest_updated") { turn.quests.push(e); continue; }
    if (e.type === "turn_started" || e.type === "turn_budget" || e.type === "player_joined") continue;
    turn = null;
    out.push({ kind: "event", e, narr: e.type === "narration" && !!d.text });
  }
  return out;
}

function turnHTML(t, latest) {
  const a = t.act.data || {}, me = mine(), you = me && a.character_id === me.id;
  const r = t.resolved?.data || {}, n = t.narr?.data || {};
  const c = S.view?.party?.find(p => p.id === a.character_id);
  const rolls = (r.steps || []).flatMap(s => s.rolls || []).map(rollHTML).join("");
  const notes = [];
  (r.steps || []).forEach(s => { if (s.error) notes.push(`<div class="ev-note bad">${esc(s.error)}</div>`); });
  (r.skipped || []).forEach(s => notes.push(`<div class="ev-note">Not this turn: ${esc(s.error)}</div>`));
  const tags = (r.steps || []).flatMap(s => s.changes || []).concat(r.world_changes || []).map(x => `<span class="tag">${esc(x)}</span>`)
    .concat(t.quests.map(q => `<span class="tag quest"><b>Quest</b>${esc(q.data?.text || q.data?.title)}</span>`));
  return `<article class="turn-card ${you ? "you" : ""} ${latest ? "latest" : ""}">
    <header><span class="avatar sm">${esc((c?.name || a.character || "?")[0])}${portrait(c)}</span><div><span class="by">${esc(you ? "You" : a.character)}</span><p>${esc(a.text)}</p></div></header>
    ${rolls ? `<div class="rolls">${rolls}</div>` : ""}${notes.join("")}
    ${n.text ? `<p class="ev-narr ${n.quick ? "quick" : ""} ${latest ? "latest" : ""}">${esc(n.text)}</p>` : ""}
    ${tags.length ? `<div class="tags">${tags.join("")}</div>` : ""}
  </article>`;
}

// pendingHTML shows the action in flight: what was said, the dice once
// they are rolled, and which step the game master is on.
function pendingHTML() {
  if (!isThinking()) return "";
  const p = S.progress, me = mine();
  const text = p?.text || S.pendingText || "";
  const who = p?.character_id ? S.view?.party?.find(c => c.id === p.character_id) : me;
  const you = who && me && who.id === me.id;
  const rolls = (p?.rolls || []).map(rollHTML).join("");
  const stage = p?.stage === "narrating" ? "Describing what happens…" : "Reading what you do…";
  return `<article class="turn-card pending ${you ? "you" : ""}">
    ${text ? `<header><span class="avatar sm">${esc((who?.name || "?")[0])}${portrait(who)}</span><div><span class="by">${esc(you ? "You" : who?.name || "")}</span><p>${esc(text)}</p></div></header>` : ""}
    ${rolls ? `<div class="rolls">${rolls}</div>` : ""}
    <div class="thinking"><span class="dots"><i></i><i></i><i></i></span>${p ? stage : "The game master is writing…"}</div>
  </article>`;
}

// isThinking: an action is in flight and its narration has not arrived.
function isThinking() {
  if (S.sending || S.progress) return true;
  for (let i = S.events.length - 1; i >= 0; i--) {
    const t = S.events[i].type;
    if (t === "narration" || t === "enemy_turns" || t === "turn_started" || t === "turn_budget" || t === "campaign_ended") return false;
    if (t === "player_action") return true;
  }
  return false;
}

function rollHTML(r) {
  if (r.kind === "encounter") return "";
  const tier = r.tier || "";
  if (r.kind === "damage" || r.kind === "heal") {
    return `<div class="die-card ${r.kind}" title="${esc(r.expr)} [${(r.dice || []).join(", ")}]"><span class="face">${r.total}</span><span class="what">${esc(r.label || r.kind)}<small>${esc(r.expr || "")}</small></span></div>`;
  }
  const word = { critical_success: "Critical!", success: "Success", failure: "Miss", critical_failure: "Fumble" }[tier] || "";
  return `<div class="die-card ${esc(tier)}" title="d20 ${r.natural} ${signed(r.mod)} = ${r.total} vs ${r.target}"><span class="face d20">${r.total}</span>
    <span class="what">${esc(r.label)}<small>d20 ${r.natural} ${signed(r.mod)} vs ${r.target}${word ? ` · ${word}` : ""}</small></span></div>`;
}

function eventHTML(e, latest) {
  const d = e.data || {}, me = mine();
  switch (e.type) {
    case "player_action": {
      const you = me && d.character_id === me.id;
      return `<div class="ev-act ${you ? "you" : ""}"><span class="avatar sm">${esc((you ? me.name : d.character || "?")[0])}${portrait(S.view?.party?.find(p => p.id === d.character_id))}</span><div><span class="by">${esc(you ? "You" : d.character)}</span><p>${esc(d.text)}</p></div></div>`;
    }
    case "action_resolved": {
      let h = "";
      const rolls = (d.steps || []).flatMap(s => s.rolls || []).map(rollHTML).join("");
      if (rolls) h += `<div class="rolls">${rolls}</div>`;
      const notes = [];
      (d.steps || []).forEach(s => { if (s.error) notes.push(`<div class="ev-note bad">${esc(s.error)}</div>`); });
      (d.skipped || []).forEach(s => notes.push(`<div class="ev-note">Not this turn: ${esc(s.error)}</div>`));
      const changes = (d.steps || []).flatMap(s => s.changes || []).concat(d.world_changes || []);
      if (changes.length) h += `<ul class="changes">${changes.map(c => `<li>${esc(c)}</li>`).join("")}</ul>`;
      return h + notes.join("");
    }
    case "narration":
      if (d.skip && !d.text) return "";
      return `<div class="narr-wrap"><p class="ev-narr ${d.quick ? "quick" : ""} ${d.opening ? "opening" : ""} ${latest ? "latest" : ""}">${esc(d.text)}</p>${S.view?.you?.joined ? `<button class="jot" data-act="jot" data-text="${esc(d.text)}" title="Copy to your notes" aria-label="Copy to your notes">${ICON.plus}</button>` : ""}</div>`;
    case "enemy_turns": {
      const rolls = (d.turns || []).map(t => t.attack?.roll ? rollHTML({ ...t.attack.roll, label: t.attack.roll.label || t.enemy }) : "").join("");
      return `${rolls ? `<div class="rolls foe">${rolls}</div>` : ""}${d.narration ? `<p class="ev-narr">${esc(d.narration)}</p>` : ""}`;
    }
    case "quest_updated": return `<div class="ev-quest"><span>Quest</span>${esc(d.text || d.title)}</div>`;
    case "location_changed": return `<div class="ev-place">${esc(d.name)}</div>`;
    case "turn_started": return `<div class="ev-sys">${me && d.character_id === me.id ? "Your turn" : `${esc(d.name)}'s turn`}</div>`;
    case "campaign_ended": return `<div class="ev-place">${d.status === "completed" ? "The story is complete" : "The story ends in failure"}</div>`;
    case "character_created": return `<div class="ev-note">${esc(d.name)} the ${esc(d.class)} joins the party</div>`;
    default: return "";
  }
}

// dock: grouped actions above the free-text box

function actionGroups() {
  const v = S.view, loc = v.location || {}, me = mine(), enc = !!v.encounter;
  const g = { look: [], talk: [], go: [], fight: [], use: [] };
  if (!enc) g.look.push({ label: "Look around", text: "I look around carefully" });
  for (const n of loc.npcs || []) if (n.alive !== false && !enc) g.talk.push({ label: n.name, sub: n.role, text: `I talk to ${n.name}: `, hint: { target_id: n.id } });
  // "known" and "open" exits can both be walked; a locked one is worth trying.
  for (const x of loc.exits || []) {
    if (x.state === "known" || x.state === "open") g.go.push({ label: x.to_name, sub: x.explored ? "explored" : "unexplored", text: enc ? `I flee toward ${x.to_name}` : `I travel to ${x.to_name}` });
    else if (x.state === "locked" && !enc) g.go.push({ label: x.to_name, sub: "locked", text: `I try to get through ${x.label || "the way to " + x.to_name}` });
  }
  for (const z of loc.zones || []) if (me && z.id !== me.zone_id) g.go.push({ label: z.name, sub: "move within", text: `I move to ${z.name}`, hint: { zone_id: z.id } });
  for (const e of v.encounter?.enemies || []) if (e.hp > 0) g.fight.push({ label: e.display, sub: `${e.hp}/${e.max_hp} HP`, text: `I attack ${e.display}`, hint: { target_id: e.id } });
  for (const i of loc.items || []) g.use.push({ label: `Take ${i.name}`, sub: i.effect, text: `I pick up the ${i.name}` });
  for (const i of me?.inventory || []) if (i.kind === "consumable") g.use.push({ label: i.name, sub: i.effect, text: `I use the ${i.name}` });
  if (me && me.hp < me.max_hp && !enc) g.use.push({ label: "Rest", sub: "recover", text: "I rest for a while" });
  return g;
}

function renderDock() {
  const v = S.view, el = $("#dock");
  if (!el) return;
  const ended = v.status === "completed" || v.status === "failed";
  if (ended) {
    el.innerHTML = `<div class="dock-inner"><div class="turn"><span class="grow">${v.status === "completed" ? "The story is complete." : "The story has ended."}</span><button class="btn small" data-act="home">Back home</button></div></div>`;
    return;
  }
  const yours = !!v.you?.your_turn, b = v.budget || {};
  // Alone at the table the turn bar says nothing new; it only appears once
  // the action is spent, to offer moving on.
  const solo = v.players.length <= 1, spent = b.action === 0;
  const pip = (name, n) => `<span class="pip ${n === 0 ? "spent" : ""}">${name}</span>`;
  const idle = v.turn_started ? Date.now() / 1000 - v.turn_started : 0;
  const groups = actionGroups();
  if (S.tray && !groups[S.tray]?.length) S.tray = "";
  const btn = (k, label, ic) => groups[k].length || k === "end" ? `<button class="act ${S.tray === k ? "on" : ""}" data-group="${k}" ${yours ? "" : "disabled"}>${ic}<span>${label}</span></button>` : "";
  const kept = $("#say")?.value ?? S.composer;
  el._groups = groups;
  el.innerHTML = `<div class="dock-inner">
    ${solo && yours ? (spent ? `<div class="turn"><span class="grow">Action used. You can still move, or go on.</span></div>` : "") : `<div class="turn">${yours
      ? `<span class="you">Your turn</span><span class="grow"></span>${pip("Action", b.action)}${pip("Move", b.move)}${b.free === -1 ? "" : pip("Free", b.free)}`
      : `<span class="grow">${v.active ? `Waiting for <b>${esc(v.active.name)}</b>` : ""}</span>${v.you?.joined && idle > 120 ? `<button class="btn small" data-act="skip">Skip their turn</button>` : ""}`}</div>`}
    ${S.tray ? `<div class="tray">${groups[S.tray].map((o, i) => `<button class="opt" data-opt="${i}"><b>${esc(o.label)}</b>${o.sub ? `<span>${esc(o.sub)}</span>` : ""}</button>`).join("")}</div>` : ""}
    <div class="acts">${btn("look", "Look", ICON.look)}${btn("talk", "Talk", ICON.talk)}${btn("go", "Go", ICON.go)}${btn("fight", "Fight", ICON.fight)}${btn("use", "Use", ICON.use)}
      ${yours && (!solo || spent) ? `<button class="act" data-act="end-turn">${ICON.end}<span>${solo ? "Go on" : "End"}</span></button>` : ""}</div>
    <form class="say" id="say-form">
      <textarea id="say" rows="1" placeholder="${yours ? "Or say exactly what you do…" : "Wait for your turn…"}" ${yours && !S.sending ? "" : "disabled"} enterkeyhint="send">${esc(kept)}</textarea>
      <button class="send" aria-label="Send" ${yours && !S.sending ? "" : "disabled"}>${S.sending ? `<span class="spinner"></span>` : ICON.send}</button>
    </form></div>`;
  const ta = $("#say");
  autosize(ta);
  ta.oninput = () => { S.composer = ta.value; autosize(ta); };
  ta.onkeydown = e => { if (e.key === "Enter" && !e.shiftKey && !matchMedia("(pointer: coarse)").matches) { e.preventDefault(); sendAction(); } };
  $("#say-form").onsubmit = e => { e.preventDefault(); sendAction(); };
}
function autosize(ta) { ta.style.height = "auto"; ta.style.height = Math.min(ta.scrollHeight, 140) + "px"; }

function fill(text, hint = {}) {
  if (!wide() && S.tab !== "story") { S.tab = "story"; S.tray = ""; renderPlay(); }
  const ta = $("#say");
  if (!ta || ta.disabled) return toast("It is not your turn yet.");
  S.hint = hint;
  ta.value = text; S.composer = text; autosize(ta);
  if (S.tray) { S.tray = ""; renderDock(); }
  const t = $("#say"); t.focus(); t.setSelectionRange(t.value.length, t.value.length);
}

async function sendAction() {
  const ta = $("#say"), text = ta?.value.trim();
  if (!text || S.sending) return;
  const hint = S.hint; S.hint = {};
  S.sending = true; S.pendingText = text; S.composer = ""; S.tray = ""; ta.value = "";
  renderDock(); renderFeed(true);
  try {
    await api("POST", `/campaigns/${S.route}/actions`, { text, ...hint });
  } catch (e) {
    toast(e.message);
    S.composer = text;
  } finally {
    S.sending = false; S.pendingText = "";
    await refreshView();
    renderFeed();
  }
}

// side column / phone tabs

function hpbar(hp, max) { const p = Math.max(0, Math.min(1, hp / max)); return `<div class="hpbar ${p <= .34 ? "low" : ""}"><i style="width:${p * 100}%"></i></div>`; }

function sceneHTML() {
  const v = S.view, l = v.location || {};
  const zname = id => l.zones?.find(z => z.id === id)?.name || "";
  const tile = (attrs, title, sub, kind) => `<button class="tile ${kind}" ${attrs}><span class="kind">${kind}</span><b>${esc(title)}</b>${sub ? `<span>${esc(sub)}</span>` : ""}</button>`;
  return `
    <section class="scene-head">${placeArt(l.id, "scene-art")}<div><h3>${esc(l.name)}</h3><p>${esc(l.description)}</p></div></section>
    ${l.zones?.length > 1 ? `<section><p class="eyebrow">Layout</p>${zoneMap()}</section>` : ""}
    ${v.encounter ? `<section><p class="eyebrow">Enemies · round ${v.encounter.round}</p><div class="tiles">${v.encounter.enemies.filter(e => e.hp > 0).map(e => tile(`data-fill="I attack ${esc(e.display)}" data-target="${esc(e.id)}"`, e.display, `${e.hp}/${e.max_hp} HP${e.ranged ? " · ranged" : ""} · ${zname(e.zone_id)}`, "foe")).join("")}</div></section>` : ""}
    ${l.npcs?.length ? `<section><p class="eyebrow">People</p><div class="tiles">${l.npcs.map(n => tile(`data-fill="I talk to ${esc(n.name)}: " data-target="${esc(n.id)}"`, n.name, `${n.role}${n.merchant ? " · sells goods" : ""}`, "person")).join("")}</div></section>` : ""}
    ${l.items?.length ? `<section><p class="eyebrow">Things</p><div class="tiles">${l.items.map(i => tile(`data-fill="I pick up the ${esc(i.name)}"`, i.name, i.effect, "item")).join("")}</div></section>` : ""}
    ${l.exits?.length ? `<section><p class="eyebrow">Ways out</p><div class="tiles">${l.exits.map(x => tile(`data-fill="${x.state !== "locked" ? `I travel to ${esc(x.to_name)}` : `I try the ${esc(x.label)}`}"`, x.to_name, `${x.label}${x.state === "locked" ? " · locked" : ""}${x.explored ? "" : " · unexplored"}`, "exit")).join("")}</div></section>` : ""}`;
}

function zoneMap() {
  const v = S.view, loc = v.location, zones = loc.zones, me = mine();
  const byId = Object.fromEntries(zones.map(z => [z.id, z]));
  const root = me?.zone_id && byId[me.zone_id] ? me.zone_id : zones[0].id;
  const dist = { [root]: 0 }, order = [root];
  for (let i = 0; i < order.length; i++) for (const a of byId[order[i]].adjacent) if (!(a in dist) && byId[a]) { dist[a] = dist[order[i]] + 1; order.push(a); }
  zones.forEach(z => { if (!(z.id in dist)) { dist[z.id] = 0; order.push(z.id); } });
  const cols = {}; order.forEach(id => (cols[dist[id]] ||= []).push(id));
  const W = 104, H = 62, GX = 22, GY = 12, pos = {};
  let rows = 1;
  Object.entries(cols).forEach(([c, ids]) => { rows = Math.max(rows, ids.length); ids.forEach((id, r) => pos[id] = { x: 4 + c * (W + GX), y: 4 + r * (H + GY) }); });
  const width = 8 + Object.keys(cols).length * (W + GX) - GX, height = 8 + rows * (H + GY) - GY;
  let s = `<svg class="zone-map" viewBox="0 0 ${width} ${height}" style="max-width:${width * 1.4}px">`;
  zones.forEach(z => z.adjacent.forEach(a => { if (a > z.id && pos[a]) s += `<line class="link" x1="${pos[z.id].x + W / 2}" y1="${pos[z.id].y + H / 2}" x2="${pos[a].x + W / 2}" y2="${pos[a].y + H / 2}"/>`; }));
  const tokens = {};
  const add = (zid, t) => (tokens[zid] ||= []).push(t);
  v.party.forEach(c => add(c.zone_id, { fill: c.id === me?.id ? "var(--accent)" : "var(--info)", name: c.name, faded: c.down }));
  (loc.npcs || []).forEach(n => add(n.zone_id, { fill: "var(--text-3)", name: n.name }));
  (v.encounter?.enemies || []).filter(e => e.hp > 0).forEach(e => add(e.zone_id, { fill: "var(--bad)", name: e.display }));
  zones.forEach(z => {
    const p = pos[z.id], short = z.name.length > 15 ? z.name.slice(0, 14) + "…" : z.name;
    s += `<g data-fill="I move to ${esc(z.name)}" data-zone="${esc(z.id)}" style="cursor:pointer"><rect class="zone ${z.id === me?.zone_id ? "mine" : ""}" x="${p.x}" y="${p.y}" width="${W}" height="${H}" rx="8"/>
      <title>${esc(z.name)}${z.desc ? ": " + esc(z.desc) : ""}</title><text x="${p.x + 8}" y="${p.y + 16}">${esc(short)}${z.cover ? " ◆" : ""}</text>`;
    (tokens[z.id] || []).forEach((t, i) => {
      const tx = p.x + 16 + (i % 4) * 22, ty = p.y + 38 + Math.floor(i / 4) * 20;
      s += `<g class="tok"><title>${esc(t.name)}</title><circle cx="${tx}" cy="${ty}" r="9" fill="${t.fill}" opacity="${t.faded ? .35 : 1}"/><text x="${tx}" y="${ty + 4}" text-anchor="middle">${esc(t.name[0])}</text></g>`;
    });
    s += `</g>`;
  });
  return s + `</svg><div class="muted small">Lime is you, blue are allies, red are enemies. Tap a zone to move.</div>`;
}

function youHTML() {
  const c = mine();
  if (!c) return `<div class="muted">You are watching this story.</div>`;
  return `
    <section class="sheet-head"><div class="avatar xl">${esc(c.name[0])}${portrait(c)}</div>
      <div class="grow"><h3>${esc(c.name)}</h3><div class="muted small">${esc(c.class)} · level ${c.level} · ${esc(c.pronouns || "they/them")}</div></div></section>
    <section class="vitals">
      <div class="vital big"><span class="k">Health</span><span class="v">${c.hp}<small>/${c.max_hp}</small></span>${hpbar(c.hp, c.max_hp)}</div>
      <div class="vital"><span class="k">Armor</span><span class="v">${c.ac}</span></div>
      <div class="vital"><span class="k">Gold</span><span class="v">${c.gold}</span></div>
      <div class="vital"><span class="k">XP</span><span class="v">${c.xp}${c.next_level_xp ? `<small>/${c.next_level_xp}</small>` : ""}</span></div>
    </section>
    <section class="statgrid">${ABIL.map(a => `<div class="stat ${c.proficient.includes(a) ? "prof" : ""}" title="${ABIL_NAME[a]}"><div class="k">${a}</div><div class="v">${signed(c.modifiers[a])}</div><div class="m">${c.scores[a]}</div></div>`).join("")}</section>
    ${c.weapon ? `<section><p class="eyebrow">Weapon</p><div class="row-item"><b>${esc(c.weapon.name)}</b><span>${esc(c.weapon.damage)} · ${esc(c.weapon.reach)}</span></div></section>` : ""}
    <section><p class="eyebrow">Carrying</p><div class="list">${c.inventory.map(i => `
      <button class="row-item" data-fill="${i.equipped ? `I use the ${esc(i.name)}` : `I ${i.kind === "consumable" ? "use" : "equip"} the ${esc(i.name)}`}"><b>${esc(i.name)}${i.equipped ? ` <span class="pill">equipped</span>` : ""}</b>${i.effect ? `<span>${esc(i.effect)}</span>` : ""}</button>`).join("") || `<div class="muted">Nothing.</div>`}</div></section>
    ${c.backstory ? `<section><p class="eyebrow">Backstory</p><p class="muted">${esc(c.backstory)}</p></section>` : ""}`;
}

// journal: a notebook with chapters. Quests, people, places and clues come
// from the world record on the server; notes are the player's own.

const CHAPTERS = [["quests", "Quests"], ["people", "People"], ["places", "Places"], ["clues", "Clues"], ["notes", "Notes"]];
const MOOD = ["hostile", "unfriendly", "wary", "neutral", "friendly", "warm", "devoted"];
const J = { tab: "quests", q: "", notes: null, notesFor: "", last: null, draft: "", editing: "", stale: false };

function loadNotes() {
  if (J.notesFor === S.route) return;
  J.notesFor = S.route; J.notes = null;
  if (!S.view?.you?.joined) { J.notes = []; return; }
  api("GET", `/campaigns/${S.route}/notes`).then(r => { J.notes = r.notes; if (sideKey() === "journal") renderSide(); }).catch(() => { J.notes = []; });
}

async function addNote(text) {
  const n = await api("POST", `/campaigns/${S.route}/notes`, { text });
  (J.notes ||= []).push(n);
  return n;
}

function journalHTML() {
  if (!S.journal) {
    api("GET", `/campaigns/${S.route}/journal`).then(j => { S.journal = j; J.last = j; if (sideKey() === "journal") renderSide(); }).catch(() => {});
  }
  loadNotes();
  const j = S.journal || (J.last?.chronicle && J.lastFor === S.route ? J.last : null);
  J.lastFor = S.route;
  const v = S.view;
  const quests = v.quests.filter(q => q.status !== "available" || q.is_main);
  const count = { quests: quests.filter(q => q.status === "active" || q.status === "completed").length, people: j?.people?.length, places: j?.places?.filter(p => p.visited).length, clues: j?.chronicle?.length, notes: J.notes?.length };
  const index = `<nav class="j-index" aria-label="Journal chapters">${CHAPTERS.map(([k, l]) => `<button class="j-tab ${J.tab === k ? "on" : ""}" data-act="jtab" data-k="${k}"><span>${l}</span><b>${count[k] ?? "·"}</b></button>`).join("")}</nav>`;
  const find = J.tab === "quests" ? "" : `<label class="j-find">${ICON.look}<input id="j-find" type="search" placeholder="Search ${J.tab}" value="${esc(J.q)}" autocomplete="off"></label>`;
  let body;
  if (J.tab === "quests") body = questsChapter(quests);
  else if (J.tab === "notes") body = notesChapter();
  else if (!j) body = `<div class="muted">Opening the journal…</div>`;
  else body = ({ people: peopleChapter, places: placesChapter, clues: cluesChapter })[J.tab](j);
  return `<div class="journal">${index}${find}<div class="j-body">${body}</div></div>`;
}

const match = (...parts) => !J.q || parts.join(" ").toLowerCase().includes(J.q.toLowerCase());
const none = what => `<div class="j-empty">${what}</div>`;

function questsChapter(quests) {
  if (!quests.length) return none("No quests yet. Talk to people and they will ask things of you.");
  return quests.map(q => `
    <div class="quest-card"><div class="row spread"><b>${esc(q.title)}</b>${q.is_main ? `<span class="pill accent">Main</span>` : `<span class="pill">${esc(q.status.replace("_", " "))}</span>`}</div>
      ${q.summary ? `<div class="muted small">${esc(q.summary)}</div>` : ""}
      ${q.objectives.map(o => `<div class="objective ${o.done ? "done" : ""}"><i></i><span>${esc(o.text)}</span></div>`).join("")}
      ${q.reward_gold || q.reward_xp ? `<div class="muted small">Reward: ${[q.reward_gold && q.reward_gold + " gold", q.reward_xp && q.reward_xp + " XP"].filter(Boolean).join(", ")}${q.giver_name ? " · from " + esc(q.giver_name) : ""}</div>` : ""}
    </div>`).join("");
}

function moodMeter(d) {
  const i = Math.max(0, Math.min(6, d + 3));
  return `<span class="mood" title="${MOOD[i]}">${MOOD.map((_, k) => `<i class="${k === i ? "on" : ""} ${k < 3 ? "neg" : k > 3 ? "pos" : ""}"></i>`).join("")}<em>${MOOD[i]}</em></span>`;
}

function peopleChapter(j) {
  const ps = j.people.filter(p => match(p.name, p.role, p.desc, p.location_name));
  if (!j.people.length) return none("Nobody yet. People you meet are written in here.");
  if (!ps.length) return none(`Nobody matches “${esc(J.q)}”.`);
  return ps.map(p => `
    <article class="dossier ${p.alive ? "" : "dead"} ${p.here ? "here" : ""}">
      <div class="mono">${esc(p.name[0])}</div>
      <div class="grow">
        <div class="d-head"><b>${esc(p.name)}</b>${p.here ? `<span class="tag here">here</span>` : ""}${p.alive ? "" : `<span class="tag">dead</span>`}${p.merchant ? `<span class="tag">trader</span>` : ""}</div>
        <div class="d-sub">${esc(p.role || "")}${p.role ? " · " : ""}${p.met ? "spoke with at" : "seen at"} ${esc(p.location_name)}</div>
        ${p.desc ? `<p>${esc(p.desc)}</p>` : ""}
        ${p.quests?.length ? `<div class="d-sub">Gave you: ${p.quests.map(esc).join(", ")}</div>` : ""}
        <div class="d-foot">${p.alive ? moodMeter(p.disposition) : ""}${p.here && p.alive && !S.view.encounter ? `<button class="btn small" data-fill="I talk to ${esc(p.name)}: " data-target="${esc(p.id)}">Talk</button>` : ""}</div>
      </div>
    </article>`).join("");
}

function dangerPips(n) { return `<span class="danger" title="danger ${n} of 5">${[1, 2, 3, 4, 5].map(k => `<i class="${k <= n ? "on" : ""}"></i>`).join("")}</span>`; }

function placesChapter(j) {
  const ps = j.places.filter(p => match(p.name, p.kind, p.hint, p.description, (p.people || []).join(" ")));
  if (!j.places.length) return none("No places yet.");
  if (!ps.length) return none(`No place matches “${esc(J.q)}”.`);
  return ps.map(p => `
    <article class="gazette ${p.visited ? "" : "rumor"} ${p.here ? "here" : ""}">
      <div class="g-head"><b>${esc(p.name)}</b>${p.here ? `<span class="tag here">you are here</span>` : p.is_hub ? `<span class="tag">home</span>` : ""}<span class="grow"></span>${dangerPips(p.danger)}</div>
      <div class="d-sub">${esc(p.kind)}${p.visited ? "" : " · not yet visited"}</div>
      ${p.visited ? (p.description ? `<p>${esc(p.description)}</p>` : "") : p.hint ? `<p class="muted">Heard of: ${esc(p.hint)}</p>` : ""}
      ${p.people?.length ? `<div class="d-sub">People: ${p.people.map(esc).join(", ")}</div>` : ""}
    </article>`).join("");
}

function cluesChapter(j) {
  const cs = j.chronicle.map((c, i) => ({ ...c, i })).filter(c => match(c.text, c.kind)).reverse();
  return `${j.summary && !J.q ? `<section class="sofar"><p class="eyebrow">The story so far</p><p>${esc(j.summary)}</p></section>` : ""}
    ${!j.chronicle.length ? none("Nothing recorded yet. Facts the story settles are written here.") : !cs.length ? none(`No clue matches “${esc(J.q)}”.`) : `<ol class="clues">${cs.map(c => `
      <li><span class="when">${c.day ? `D${c.day}` : `T${c.turn}`}</span><p>${esc(c.text)}${c.kind ? ` <span class="tag">${esc(c.kind)}</span>` : ""}</p>
        <button class="jot" data-act="jot" data-text="${esc(c.text)}" title="Copy to your notes" aria-label="Copy to your notes">${ICON.plus}</button></li>`).join("")}</ol>`}`;
}

function notesChapter() {
  if (!S.view.you?.joined) return none("Join this story to keep notes.");
  if (!J.notes) return `<div class="muted">Opening your notes…</div>`;
  const ns = J.notes.filter(n => match(n.text)).slice().reverse();
  const stamp = n => `Day ${n.day || 1} · ${esc(n.time || "")}`;
  return `
    <form class="n-new" id="n-new"><textarea id="n-text" class="textarea" rows="3" placeholder="Write it down before you forget: names, hunches, what someone wasn't telling you…">${esc(J.draft)}</textarea>
      <div class="row"><span class="muted small grow">Only you can read these.</span><button class="btn small primary">Write</button></div></form>
    ${!J.notes.length ? none("No notes yet. Tap + on a line of the story or a clue to copy it here.") : !ns.length ? none(`No note matches “${esc(J.q)}”.`) : `<div class="notes">${ns.map(n => J.editing === n.id ? `
      <form class="note editing" data-note="${esc(n.id)}"><div class="n-stamp">${stamp(n)}</div><textarea class="textarea" id="n-edit" rows="4">${esc(n.text)}</textarea>
        <div class="row"><button type="button" class="btn small ghost" data-act="note-del" data-id="${esc(n.id)}">Delete</button><span class="grow"></span><button type="button" class="btn small ghost" data-act="note-cancel">Cancel</button><button class="btn small primary">Save</button></div></form>` : `
      <button class="note" data-act="note-edit" data-id="${esc(n.id)}"><div class="n-stamp">${stamp(n)}</div><p>${esc(n.text)}</p></button>`).join("")}</div>`}`;
}

// The side panel re-renders on every game event. Hold off while the player
// is typing in the journal and catch up when they leave the field.
function journalBusy() {
  const a = document.activeElement;
  return !!a && a.closest?.(".journal") && a.matches("input, textarea");
}
document.addEventListener("focusout", () => setTimeout(() => { if (J.stale && !journalBusy()) { J.stale = false; renderSide(); } }, 0));
document.addEventListener("input", e => {
  if (e.target.id === "j-find") {
    J.q = e.target.value;
    const pos = e.target.selectionStart;
    renderJournalBody();
    const f = $("#j-find"); if (f) { f.focus(); f.setSelectionRange(pos, pos); }
  }
  if (e.target.id === "n-text") J.draft = e.target.value;
});
function renderJournalBody() {
  const el = $(".journal");
  if (!el) return;
  const tmp = document.createElement("div");
  tmp.innerHTML = journalHTML();
  el.querySelector(".j-body").replaceWith(tmp.querySelector(".j-body"));
  el.querySelector(".j-index").replaceWith(tmp.querySelector(".j-index"));
}
document.addEventListener("submit", async e => {
  const f = e.target;
  if (f.id === "n-new") {
    e.preventDefault();
    const text = $("#n-text").value.trim();
    if (!text) return;
    try { await addNote(text); J.draft = ""; renderSide(); } catch (er) { toast(er.message); }
  } else if (f.dataset?.note) {
    e.preventDefault();
    const id = f.dataset.note, text = $("#n-edit").value.trim();
    if (!text) return;
    try {
      await api("POST", `/campaigns/${S.route}/notes/${id}`, { text });
      const n = J.notes.find(n => n.id === id); if (n) n.text = text;
      J.editing = ""; renderSide();
    } catch (er) { toast(er.message); }
  }
});
document.addEventListener("click", async e => {
  const t = e.target.closest("[data-act]");
  if (!t) return;
  switch (t.dataset.act) {
    case "jtab": J.tab = t.dataset.k; J.q = ""; J.editing = ""; return renderSide();
    case "note-edit": J.editing = t.dataset.id; renderSide(); return $("#n-edit")?.focus();
    case "note-cancel": J.editing = ""; return renderSide();
    case "note-del":
      try {
        await api("DELETE", `/campaigns/${S.route}/notes/${t.dataset.id}`);
        J.notes = J.notes.filter(n => n.id !== t.dataset.id); J.editing = ""; renderSide();
      } catch (er) { toast(er.message); }
      return;
    case "jot":
      if (!S.view?.you?.joined) return toast("Join this story to keep notes.");
      loadNotes();
      try { await addNote(`“${t.dataset.text.slice(0, 3900)}”`); t.classList.add("done"); toast("Copied to your notes."); if (sideKey() === "journal" && J.tab === "notes") renderSide(); } catch (er) { toast(er.message); }
      return;
  }
});

function mapHTML() {
  if (!S.map) {
    api("GET", `/campaigns/${S.route}/map`).then(m => { S.map = m; if (sideKey() === "map") renderSide(); }).catch(() => {});
    return `<div class="muted">Unfolding the map…</div>`;
  }
  const m = S.map, here = m.current_id;
  const byId = Object.fromEntries(m.locations.map(l => [l.id, l]));
  const near = new Map();
  m.edges.forEach(e => { if (e.a === here) near.set(e.b, e.state); if (e.b === here) near.set(e.a, e.state); });
  const others = m.locations.filter(l => l.id !== here && !near.has(l.id));
  const row = (l, extra = "", fillText = "") => `<${fillText ? `button data-fill="${esc(fillText)}"` : "div"} class="row-item"><b>${esc(l.name)}${l.id === m.hub_id ? ` <span class="pill">home</span>` : ""}</b><span>${esc(l.kind || "")}${extra}</span></${fillText ? "button" : "div"}>`;
  return `
    <section><p class="eyebrow">You are here</p>${byId[here] ? `<div class="here-card">${placeArt(here, "scene-art")}${row(byId[here])}</div>` : ""}</section>
    ${near.size ? `<section><p class="eyebrow">Paths from here</p><div class="list">${[...near].map(([id, st]) => byId[id] ? row(byId[id], st === "locked" ? " · locked" : byId[id].visited ? " · visited" : " · not yet visited", st === "locked" ? "" : `I travel to ${byId[id].name}`) : "").join("")}</div></section>` : ""}
    ${others.length ? `<section><p class="eyebrow">Known places</p><div class="list">${others.map(l => row(l, l.visited ? " · visited" : "")).join("")}</div></section>` : ""}`;
}

function sideKey() { return wide() ? S.panel : (S.tab === "story" ? S.panel : S.tab); }

// ---- actions

let lastWide = wide();
addEventListener("resize", () => { if (wide() !== lastWide) { lastWide = wide(); if ($("#hud")) renderPlay(); } });

document.addEventListener("click", async e => {
  const t = e.target.closest("[data-act],[data-open],[data-premise],[data-kit],[data-prof],[data-panel],[data-tab],[data-group],[data-opt],[data-step],[data-fill]");
  if (!t) return;
  const act = t.dataset.act;
  if (t.dataset.open) return go(`#/c/${t.dataset.open}`);
  if (t.dataset.premise) { $("#premise").value = t.dataset.premise; return; }
  if (t.dataset.kit) { readEditor().kit = t.dataset.kit; return renderSetup(); }
  if (t.dataset.step) { readEditor(); S.step = +t.dataset.step; return renderSetup(); }
  if (t.dataset.prof) {
    const d = readEditor(), a = t.dataset.prof, p = d.proficient || (d.proficient = []);
    if (p.includes(a)) p.splice(p.indexOf(a), 1);
    else { p.push(a); if (p.length > 2) p.shift(); }
    return renderSetup();
  }
  if (t.dataset.tab) {
    if (wide()) { if (t.dataset.tab !== "story") { S.panel = t.dataset.tab; renderSide(); } return; }
    S.tab = t.dataset.tab; if (S.tab !== "story") S.panel = S.tab;
    return renderPlay();
  }
  if (t.dataset.panel) { S.panel = t.dataset.panel; if (!wide()) S.tab = t.dataset.panel; return wide() ? renderSide() : renderPlay(); }
  if (t.dataset.group) {
    const g = t.dataset.group, opts = $("#dock")._groups[g];
    if (opts.length === 1 && g === "look") return fill(opts[0].text, opts[0].hint);
    S.tray = S.tray === g ? "" : g;
    return renderDock();
  }
  if (t.dataset.opt) { const o = $("#dock")._groups[S.tray][+t.dataset.opt]; return fill(o.text, o.hint); }
  if (t.dataset.fill) {
    const hint = t.dataset.target ? { target_id: t.dataset.target } : t.dataset.zone ? { zone_id: t.dataset.zone } : {};
    return fill(t.dataset.fill, hint);
  }
  try {
    switch (act) {
      case "signout": await fetch("/auth/logout", { method: "POST" }); S.me = null; leaveCampaign(); S.route = ""; location.hash = ""; return renderSignin();
      case "home": return go("#/");
      case "compose": S.composing = true; return renderHome();
      case "compose-close": S.composing = false; return renderHome();
      case "create": return createCampaign();
      case "join":
        t.disabled = true;
        await api("POST", `/campaigns/${S.route}/players`, { name: S.me.name });
        return refreshView();
      case "draft": {
        const concept = $("#concept").value.trim() || $("#concept").placeholder.replace(/^e\.g\. /, "");
        t.disabled = true; t.innerHTML = `<span class="spinner"></span> Drafting…`;
        try {
          const r = await api("POST", `/campaigns/${S.route}/characters/draft`, { concept });
          S.draft = { ...r.character, concept, proficient: r.character.proficient || [] };
          S.step = 0;
        } finally { renderSetup(); }
        return;
      }
      case "edit": {
        const c = mine();
        S.draft = { name: c.name, class: c.class, kit: c.kit, pronouns: c.pronouns, backstory: c.backstory || "", concept: c.concept || "", scores: { ...c.scores }, proficient: [...c.proficient] };
        S.step = 0;
        return renderSetup();
      }
      case "step-next": readEditor(); S.step = Math.min(STEPS.length - 1, S.step + 1); return renderSetup();
      case "step-back": readEditor(); S.step = Math.max(0, S.step - 1); return renderSetup();
      case "cancel-draft": S.draft = null; return renderSetup();
      case "save-char": {
        const d = readEditor();
        t.disabled = true;
        try {
          await api("POST", `/campaigns/${S.route}/characters`, { character: d });
          S.draft = null;
          await refreshView();
          renderSetup();
        } catch (er) { $("#d-err").textContent = er.message; t.disabled = false; }
        return;
      }
      case "start":
        t.disabled = true; t.innerHTML = `<span class="spinner"></span> Setting the scene…`;
        await api("POST", `/campaigns/${S.route}/start`);
        return refreshView();
      case "end-turn": return api("POST", `/campaigns/${S.route}/skip`, {});
      case "skip": return api("POST", `/campaigns/${S.route}/skip`, {});
    }
  } catch (er) { toast(er.message); if (S.view?.status === "lobby") renderSetup(); }
});

// ---- boot

(async () => {
  try {
    const r = await fetch("/auth/me");
    if (r.ok) S.me = await r.json();
  } catch {}
  route();
})();
