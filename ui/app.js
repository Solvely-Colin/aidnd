// aidnd player UI. No build step: plain modules, template strings and one
// delegated click handler. Every call goes to /api/* on the Worker, which
// adds the player's id from the session cookie.
//
// Layout: under 1100px the play screen is a column (banner, story, dock)
// with panels in a bottom sheet; from 1100px it is a stage, with the place
// art on the left, the story column on the right and panels sliding over
// the art from the left.

const $ = (s, el = document) => el.querySelector(s);
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const app = $("#app");

const svg = (d, w = 2) => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`;
const QUILL = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" aria-hidden="true"><path d="M4 20c4-1 9-6 15-16-6 3-11 9-15 16zM4 20l6-6M9 10.5l4.5 1"/></svg>`;
const ICON = {
  back: svg(`<path d="M15 18l-6-6 6-6"/>`),
  close: svg(`<path d="M6 6l12 12M18 6L6 18"/>`, 2.2),
  menu: svg(`<path d="M4 7h16M4 12h16M4 17h10"/>`),
  send: svg(`<path d="M5 12h14M13 6l6 6-6 6"/>`, 2.4),
  plus: svg(`<path d="M12 5v14M5 12h14"/>`),
};
// The mark is an open eye over a line of sounding: someone keeping watch on the water.
const EYE = `<svg viewBox="0 0 34 34" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><path d="M3 17c5-8 23-8 28 0-5 8-23 8-28 0z"/><circle cx="17" cy="17" r="5"/><path d="M17 22v9M13 27c2 2.5 6 2.5 8 0M17 3v7"/></svg>`;
const mark = (cls = "") => `<span class="mark ${cls}">${EYE}</span>`;
const ROMAN = n => { let s = ""; for (const [v, r] of [[1000, "M"], [900, "CM"], [500, "D"], [400, "CD"], [100, "C"], [90, "XC"], [50, "L"], [40, "XL"], [10, "X"], [9, "IX"], [5, "V"], [4, "IV"], [1, "I"]]) while (n >= v) { s += r; n -= v; } return s; };
// tally draws hit points as strokes in a ledger, struck through in red as
// they are lost. Big pools count several points to a stroke.
function tally(hp, max, label = "") {
  max = Math.max(1, max); hp = Math.max(0, Math.min(max, hp));
  const per = Math.ceil(max / 30), n = Math.ceil(max / per), left = Math.ceil(hp / per);
  const strokes = Array.from({ length: n }, (_, i) => `<i class="${i < left ? "" : "x"}${(i + 1) % 5 === 0 ? " g" : ""}"></i>`).join("");
  return `<span class="tally" role="img" aria-label="${esc(label)}${label ? ": " : ""}${hp} of ${max} left">${strokes}</span>`;
}

const S = {
  me: null,          // {player_id, name}
  meta: null,
  route: "",
  view: null,        // campaign view
  events: [],
  lastId: 0,
  es: null,
  progress: null,    // how far the action in flight has got (unsaved stream event)
  sending: false,
  pendingText: "",
  draft: null,       // character being edited
  step: 0,           // builder step
  swap: "",          // builder: ability picked for a swap
  era: "",           // home: setting picked for a new story
  writing: 0,        // home: when the new story was sent
  panel: null,       // scene | talk | sheet | journal | map
  lastPanel: "scene",
  sheetFor: "",
  talkTo: "",        // npc id while in conversation
  asked: {},         // npc id + topic -> true
  tray: "",
  hint: {},
  composer: "",
  collapsed: false,  // phone banner
  open: {},          // roll id -> breakdown shown
  journal: null,
  map: null,
  stock: {},         // npc id -> wares
};

const stageMode = () => innerWidth >= 1100;
const sideMode = () => innerWidth >= 840;

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
  toastTimer = setTimeout(() => el.remove(), 2600);
}

// ---- art. A small landscape drawn from an id sits under every picture, so
// a story has art before (or without) image generation.

function hash(s) { let h = 2166136261; for (const c of String(s)) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; }
const COVERS = [
  ["#0c1418", "#14232a", "#1d343d", "#7fe0d0"],
  ["#0f1216", "#1a2027", "#28323c", "#a3b0b8"],
  ["#14100f", "#231b19", "#36292a", "#ef5f5a"],
  ["#0b1112", "#122022", "#1b3234", "#a8efe3"],
];
function drawn(seed) {
  let h = hash(seed);
  const rnd = () => { h = Math.imul(h ^ (h >>> 15), 2246822507) >>> 0; return (h % 1000) / 1000; };
  const [sky, far, near, glow] = COVERS[hash(seed) % COVERS.length];
  const ridge = (base, amp, n) => {
    let d = `M0 ${base}`;
    for (let i = 1; i <= n; i++) d += ` L${(i * 400) / n} ${base - rnd() * amp}`;
    return d + " L400 300 L0 300Z";
  };
  const mx = 60 + rnd() * 280, my = 40 + rnd() * 50, mr = 14 + rnd() * 18;
  const stars = Array.from({ length: 16 }, () => `<circle cx="${(rnd() * 400).toFixed(0)}" cy="${(rnd() * 120).toFixed(0)}" r="${(0.5 + rnd()).toFixed(1)}" fill="${glow}" opacity="${(0.2 + rnd() * 0.5).toFixed(2)}"/>`).join("");
  return `<svg viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
    <rect width="400" height="300" fill="${sky}"/>${stars}
    <circle cx="${mx}" cy="${my}" r="${mr}" fill="${glow}" opacity=".85"/>
    <path d="${ridge(190, 70, 7)}" fill="${far}"/>
    <path d="${ridge(235, 50, 11)}" fill="${near}"/>
  </svg>`;
}

// Generated art: the Worker paints a cover per story, a portrait per
// character and a picture of the place the party stands in, the first time
// one is asked for. Until it exists the image fails to load, the drawn art
// or the initial underneath stays in view, and we ask again a few times.
const ART = new Map(); // src -> "ok" | "gone" | "wait" | tries so far
const ART_TRIES = 15, ART_WAIT = 6000;
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

const coverArt = id => `<div class="art">${drawn(id)}${artImg(`/img/${id}/cover`)}</div>`;
// Only the place the party stands in can be painted (the Worker reads it
// from the campaign view); other places keep their drawn art.
const placeArt = id => `<div class="art">${drawn(id || "place")}${id && id === S.view?.location?.id ? artImg(`/img/${S.route}/place/${id}`) : ""}</div>`;
const TITLES = /^((the|mr|mrs|miss|ms|dr|sir|lady|lord|constable|captain|father|mother|sister|brother|elder|old)\.?\s+)+/i;
const firstName = s => String(s || "").replace(TITLES, "").split(" ")[0] || String(s || "");
const initial = s => esc((firstName(s)[0] || "?").toUpperCase());
function face(c, cls = "") {
  const img = c?.id && S.route && S.view?.party?.some(p => p.id === c.id) ? artImg(`/img/${S.route}/char/${c.id}`) : "";
  return `<span class="face ${cls}">${initial(c?.name)}${img}</span>`;
}

// ---- routing

function go(h) { if (location.hash !== h) location.hash = h; else route(); }
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
  Object.assign(S, {
    es: null, view: null, events: [], lastId: 0, progress: null, draft: null, step: 0, swap: "", panel: null, sheetFor: "",
    talkTo: "", asked: {}, tray: "", hint: {}, composer: "", collapsed: false, open: {}, journal: null, map: null, stock: {}, letterOpen: false, summonsGone: "",
  });
  J.notesFor = ""; J.notes = null; J.tab = "quests"; J.q = "";
  DIE.queue = []; DIE.cur = null;
}

// ---- sign in

function renderSignin(err = "") {
  app.innerHTML = `
  <div class="scrollpage"><div class="signin">
    <div class="signin-art"><div class="art">${drawn("aidnd-sign-in")}</div></div>
    <div class="signin-form"><form id="signin">
      <div class="signin-head"><span class="brand">${mark()}aidnd</span><p>Tell the table what you do. The dice decide what happens.</p></div>
      <div class="signin-fields">
        <label class="field"><span>Your name</span><input class="under" name="name" autocomplete="nickname" maxlength="40" required value="${esc(localName())}"></label>
        <label class="field"><span>Play code</span><input class="under code" name="code" type="password" autocomplete="current-password" required>
          <span class="help">Your whole table signs in with the same code.</span></label>
      </div>
      <div class="error">${esc(err)}</div>
      <button class="btn primary block">Sit down at the table</button>
    </form></div>
  </div></div>`;
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

const SUGGESTIONS = [
  "a 1930s detective story in a fog-bound harbor town",
  "a heist in a floating city of clockwork and canals",
  "survivors of a shipwreck on an island that remembers them",
  "a frontier town where the dead keep coming back to vote",
];
const SETTING_WORD = { fantasy: "Fantasy", gaslight: "Gaslight", pulp: "1930s pulp", modern: "Modern", future: "Sci-fi" };

async function renderHome() {
  S.writing = 0;
  if (!S.meta) api("GET", "/meta").then(m => {
    S.meta = m;
    if (S.route || S.writing) return;
    if ($("#eras")) $("#eras").outerHTML = erasHTML();
    if ($("#sagas")) $("#sagas").innerHTML = sagasHTML();
  }).catch(() => {});
  app.innerHTML = `<div class="scrollpage" id="home"><div class="home-body top"><section><div class="shelf">${`<div class="book skeleton"><div class="cov"></div></div>`.repeat(3)}</div></section></div></div>`;
  let mine, all;
  try {
    [mine, all] = await Promise.all([api("GET", "/me/campaigns"), api("GET", "/campaigns")]);
  } catch (e) { $("#home").innerHTML = `<div class="center-msg"><div class="error">${esc(e.message)}</div></div>`; return; }
  if (S.route) return;
  const order = { active: 0, lobby: 1, completed: 2, failed: 2 };
  const stories = mine.campaigns.slice().sort((a, b) => (order[a.status] ?? 3) - (order[b.status] ?? 3) || b.updated_at - a.updated_at);
  const lead = stories.find(c => c.your_turn) || stories.find(c => c.status === "active" || c.status === "lobby");
  const rest = stories.filter(c => c !== lead);
  const mineIds = new Set(stories.map(c => c.id));
  // a story under way with a free seat can be joined too: the newcomer is
  // written in where the party stands
  const open = all.campaigns.filter(c => (c.status === "lobby" || c.status === "active") && !mineIds.has(c.id) && c.players < c.max_players);
  const top = `<div class="hero-top"><span class="brand">${mark()}aidnd</span><button class="chip-scrim" data-act="signout">${esc(S.me.name)} · Sign out</button></div>`;
  $("#home").innerHTML = `
    ${lead ? `<div class="hero">
      <div class="home-art">${coverArt(lead.id)}</div>${top}
      <div class="home-lead"><div class="col">
        <span class="eyebrow-acc">${lead.your_turn ? `Your turn${lead.deadline ? ` · autopilot in ${untilText(lead.deadline)}` : ""}` : lead.status === "lobby" ? "Gathering the party" : "In play"} · ${lead.players}/${lead.max_players} players · ${esc(ago(lead.updated_at))}</span>
        <h1 class="title-xl">${esc(lead.title)}</h1>
        ${lead.pitch ? `<p class="lede">${esc(clip(lead.pitch, 180))}</p>` : ""}
        <div class="hero-cta"><button class="btn primary" data-open="${esc(lead.id)}">${lead.status === "lobby" ? "Back to the table" : lead.your_turn ? "Take your turn" : "Continue the story"}</button></div>
      </div></div>
    </div>` : `<div class="hero" style="height:64px">${top}</div>`}
    <div class="home-body">
      ${rest.length ? `<section><span class="mono-k">Your other stories</span><div class="shelf">${rest.map(c => bookHTML(c)).join("")}</div></section>` : ""}
      ${open.length ? `<section><span class="mono-k">Open tables</span><div class="shelf">${open.map(c => bookHTML(c, true)).join("")}</div></section>` : ""}
      <section id="sagas">${sagasHTML()}</section>
      <section id="new">${newStoryHTML()}</section>
    </div>`;
}

function bookHTML(c, open) {
  const meta = { lobby: "Gathering the party", active: open ? "Under way · join in" : c.your_turn ? "Your turn" : `In play · ${ago(c.updated_at)}`, completed: "Finished", failed: "Lost" }[c.status] || "";
  const seats = c.status === "lobby" || (open && c.status === "active");
  return `<button class="book" data-open="${esc(c.id)}"><div class="cov">${coverArt(c.id)}</div><b>${esc(c.title)}</b><span>${esc(meta)}${seats ? ` · ${c.players}/${c.max_players}` : ""}</span></button>`;
}

function erasHTML() {
  const list = [["", "Any"], ...(S.meta?.settings || []).map(s => [s.key, SETTING_WORD[s.key] || s.name])];
  return `<div class="eras" id="eras">${list.map(([k, l]) => `<button class="${S.era === k ? "on" : ""}" data-era="${esc(k)}">${esc(l)}</button>`).join("")}</div>`;
}

function deathHTML() {
  const opts = [["standard", "Characters can die"], ["story", "Heroes never die"]];
  return `<div class="eras" id="death">${opts.map(([k, l]) => `<button class="${(S.lethality || "standard") === k ? "on" : ""}" data-lethal="${k}">${l}</button>`).join("")}</div>`;
}

// Strain is the horror track: six marks, a breakdown at four, a scar at six.
function strainHTML(st) {
  // each mark of strain is a pane, cracked once it is taken
  const panes = Array.from({ length: st.max }, (_, i) => `<i class="${i < st.value ? "on" : ""}${i >= 3 ? " hi" : ""}"><svg viewBox="0 0 30 14" aria-hidden="true"><path d="M2 8l6-4 4 7 6-8 5 6 5-3"/></svg></i>`).join("");
  return `<section class="strain"><p class="sh">Strain · ${st.value} of ${st.max}</p>
    <div class="strain-track" role="img" aria-label="Strain ${st.value} of ${st.max}">${panes}</div>
    <span class="small-note">${st.shaken ? "Shaken: disadvantage on saves against fear. " : ""}At four the mind gives way; at six it leaves a scar. Rest somewhere safe to mend.</span>
    ${st.scars?.length ? `<div class="scars">${st.scars.map(s => `<div><b>${esc(s.name)}</b><span>${esc(s.text)}</span></div>`).join("")}</div>` : ""}</section>`;
}

// Sagas are hand-written long campaigns in one open world that grows as
// players travel: regions are built the first time someone goes there.
function sagasHTML() {
  const list = S.meta?.sagas || [];
  if (!list.length) return "";
  return `<span class="mono-k">A long campaign</span>${list.map(s => `<div class="saga">
      <div class="saga-art"><div class="art">${drawn("saga-" + s.key)}</div></div>
      <div class="saga-txt">
        <span class="eyebrow-acc">${esc(SETTING_WORD[s.setting] || s.setting)} · an open world · begins in ${esc(s.start)}</span>
        <h2 class="title-l">${esc(s.title)}</h2>
        <p class="pitch">${esc(s.pitch)}</p>
        <div class="saga-foot"><button class="btn primary" data-saga="${esc(s.key)}">Begin the campaign</button>
          <span class="help">One world that grows as you explore it, played over weeks. Go solo or bring friends. Horror leaves marks: watch your strain.</span></div>
      </div></div>`).join("")}`;
}

async function startSaga(key) {
  S.writing = Date.now();
  const box = $("#sagas");
  if (box) box.innerHTML = `<div class="writing"><h3>Opening the world</h3><span class="help">Laying out the first town and its people…</span></div>`;
  try {
    const c = await api("POST", "/campaigns", { saga: key, lethality: S.lethality || "standard" });
    await api("POST", `/campaigns/${c.id}/players`, { name: S.me.name });
    go(`#/c/${c.id}`);
  } catch (e) { toast(e.message); renderHome(); }
  finally { S.writing = 0; }
}

function newStoryHTML() {
  const sug = SUGGESTIONS[hash(S.me?.name || "") % SUGGESTIONS.length];
  return `<span class="mono-k">Begin something new</span>
    <textarea class="under big" id="premise" rows="2" placeholder="What is this story about? A place, a mood, a mystery…"></textarea>
    ${erasHTML()}
    ${deathHTML()}
    <div class="new-foot"><button class="try" data-premise="${esc(sug)}">Try: <em>${esc(sug)}</em></button><button class="btn" data-act="create">Write this story</button></div>
    <span class="help">The DM writes the places, the people and a main quest. It takes about a minute. To join a friend’s story, open it from Open tables, even once it is under way.</span>`;
}

// The world is written in one long request, so the steps are paced by the
// clock, not reported by the server.
const WRITE_STEPS = ["Places and paths", "People and what they want", "The main quest", "Cover art"];
function writingHTML() {
  const t = (Date.now() - S.writing) / 1000, at = Math.min(3, Math.floor(t / 14));
  return `<div class="writing"><h3>The DM is writing your world</h3><div class="steps-list">${WRITE_STEPS.map((l, i) =>
    `<div class="${i < at ? "done" : i === at ? "now" : ""}"><i>${i < at ? "✓" : i === at ? "…" : "·"}</i><span>${l}</span></div>`).join("")}</div>
    <span class="help">${Math.round(t)}s</span></div>`;
}

function clip(s, n) { s = String(s || ""); return s.length > n ? s.slice(0, n - 1).replace(/\s+\S*$/, "") + "…" : s; }
function ago(ts) {
  if (!ts) return "";
  const s = Math.max(1, Date.now() / 1000 - ts);
  if (s < 3600) return `${Math.round(s / 60) || 1} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}

async function createCampaign() {
  const el = $("#premise");
  let premise = el.value.trim();
  if (!premise) { el.focus(); return toast("Write a line about the story first."); }
  // The author picks the setting from an era word in the premise.
  if (S.era) premise += ` Setting: ${SETTING_WORD[S.era] || S.era}.`;
  S.writing = Date.now();
  const box = $("#new");
  box.innerHTML = writingHTML();
  const t = setInterval(() => { if ($("#new") === box) box.innerHTML = writingHTML(); }, 1000);
  try {
    const c = await api("POST", "/campaigns", { premise, lethality: S.lethality || "standard" });
    await api("POST", `/campaigns/${c.id}/players`, { name: S.me.name });
    go(`#/c/${c.id}`);
  } catch (e) { toast(e.message); renderHome(); }
  finally { clearInterval(t); S.writing = 0; }
}

// ---- campaign

async function openCampaign(id) {
  if (!S.view) app.innerHTML = `<div class="center-msg"><span class="spinner" style="color:var(--ink3)"></span></div>`;
  try {
    S.view = await api("GET", `/campaigns/${id}`);
  } catch (e) {
    app.innerHTML = `<div class="center-msg"><div class="box"><div class="error">${esc(e.message)}</div><button class="btn" data-act="home">Back home</button></div></div>`;
    return;
  }
  if (!S.meta) api("GET", "/meta").then(m => { S.meta = m; if (S.view?.status === "lobby") render(); }).catch(() => {});
  connect(id);
  render();
}

function restartFeed() {
  S.es?.close(); S.es = null;
  S.events = []; S.lastId = 0; S.open = {}; S.dmHist = null;
  connect(S.route);
  refreshView();
}
function connect(id) {
  if (S.es) return;
  const es = new EventSource(`/api/campaigns/${id}/events`);
  const types = ["campaign_created", "player_joined", "character_created", "player_action", "action_resolved", "narration", "enemy_turns", "quest_updated", "location_changed", "turn_started", "turn_budget", "campaign_ended", "conditions", "action_pending", "action_cancelled", "death_save", "rewound", "dm_action", "group_split", "group_joined", "world_tick", "downtime_done", "world_event", "vision", "catch_up"];
  types.forEach(t => es.addEventListener(t, ev => onEvent(JSON.parse(ev.data))));
  // Progress events are not saved and have no id: they say the action is
  // being read, then carry its dice while the narration is written.
  es.addEventListener("progress", ev => {
    S.progress = JSON.parse(ev.data).data;
    // another group's turn is theirs to watch
    if (S.progress?.character_id && !actorName(S.progress.character_id)) { S.progress = null; return; }
    (S.progress?.rolls || []).forEach(r => queueRoll(r, actorName(S.progress.character_id)));
    renderFeed();
  });
  S.es = es;
}

let refreshTimer, feedTimer;
function onEvent(e) {
  if (e.id <= S.lastId) return;
  // A rewind hides the events since; start the feed again from the top.
  if (e.type === "rewound" && fresh(e)) { toast(e.data?.text || "The story was rewound."); return restartFeed(); }
  S.lastId = e.id;
  S.events.push(e);
  if (e.type !== "character_created" && e.type !== "player_joined") S.progress = null;
  if (fresh(e)) liveEvent(e);
  clearTimeout(feedTimer);
  feedTimer = setTimeout(renderFeed, 30);
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refreshView, 250);
}

// myEvents keeps what happened to the caller's group: when the world is
// split, each group has its own story. It follows the caller through splits
// and joins, so their story before a split stays in their feed.
function myEvents(evs) {
  const me = S.view?.you?.character_id;
  let cur = "main";
  const out = [];
  for (const e of evs) {
    const d = e.data || {};
    if (e.type === "group_split") { if (d.character_id === me) cur = d.group; out.push(e); continue; }
    if (e.type === "group_joined") { if (d.from === cur || d.into === cur) cur = d.group; out.push(e); continue; }
    if (e.type === "character_created" && d.player_id === S.me?.player_id) cur = S.view?.group || cur;
    if (!d.party || d.party === cur) out.push(e);
  }
  return out;
}

// liveEvent reacts to something that just happened (not to the replay).
function liveEvent(e) {
  const d = e.data || {};
  if (d.party && S.view?.group && d.party !== S.view.group) return;
  if (e.type === "action_resolved") eachRoll(d).forEach(r => queueRoll(r.d20, actorName(d.character_id)));
  if (e.type === "enemy_turns") (d.turns || []).forEach(t => t.attack?.roll && queueRoll({ ...t.attack.roll, kind: "attack", label: t.attack.roll.label || `Attack on ${t.target || "the party"}` }, t.enemy, true));
  if (e.type === "location_changed") { S.collapsed = false; S.talkTo = ""; S.stock = {}; }
  if (e.type === "turn_started" && d.player_id !== S.me?.player_id) S.tray = "";
  if (e.type === "death_save" && d.roll) queueRoll(d.roll, rollerName(d.roll.label));
}

// rollerName finds the party member whose name starts a roll's label.
function rollerName(label) {
  const c = (S.view?.party || []).find(p => String(label || "").startsWith(p.name + " "));
  return c?.name || "";
}

const FRESH_SECS = 30;
const fresh = e => e.created_at && Date.now() / 1000 - e.created_at < FRESH_SECS;

async function refreshView() {
  if (!S.route) return;
  const before = S.view?.status;
  try { S.view = await api("GET", `/campaigns/${S.route}`); } catch { return; }
  // the tab says when it's your turn, for a player who left it open
  document.title = (!S.view.free_roam && S.view.you?.your_turn ? "Your turn · " : "") + BASE_TITLE;
  S.journal = null; S.map = null;
  if (S.view.encounter && S.talkTo) S.talkTo = "";
  if (before !== S.view.status) return render();
  if (S.view.status === "lobby" || joining()) return renderLobby(true);
  renderChrome();
}

function render() {
  if (!S.view) return;
  if (S.view.status === "lobby" || joining()) return renderLobby();
  if (S.draft && mine()?.dead) return renderBuilder();
  renderPlay();
}

// joining: the story is under way and the caller has no character in it
// yet (they haven't taken a seat, or are still making one).
const joining = () => S.view?.status === "active" && !S.view.you?.character_id;

function mine() {
  const v = S.view;
  return v?.you?.character_id ? v.party.find(c => c.id === v.you.character_id) : null;
}
function actorName(cid) { return S.view?.party?.find(c => c.id === cid)?.name || ""; }
const isMe = cid => !!cid && cid === S.view?.you?.character_id;
const playerOf = c => S.view?.players?.find(p => p.character_id === c?.id || p.id === c?.player_id);

// ---- lobby

const ABIL = ["STR", "DEX", "CON", "INT", "WIS", "CHA"];
const ABIL_NAME = { STR: "Strength", DEX: "Dexterity", CON: "Constitution", INT: "Intelligence", WIS: "Wisdom", CHA: "Charisma" };
const mod = n => Math.floor((n - 10) / 2);
const signed = n => (n >= 0 ? "+" : "−") + Math.abs(n);
// Kits are reskinned per campaign setting (a 1930s game gets pistols, not swords).
function kitList() { return S.meta?.settings?.find(s => s.key === S.view?.setting)?.kits || S.meta?.kits || []; }
function kit(k) { return kitList().find(x => x.key === k); }
const kitName = k => kit(k)?.name || k || "";
// roleName is what a character does in this era: their class, or for old
// sheets their kit.
const roleName = c => c?.build?.class_name || kitName(c?.kit);
// roleLine is "Occultist · Human · Professor", with a custom title first.
function roleLine(c) {
  const b = c.build;
  if (!b) return [c.class, kitName(c.kit)].filter(Boolean).join(" · ");
  const title = c.class && c.class.toLowerCase() !== b.class_name.toLowerCase() ? c.class : "";
  return [title, b.class_name + (b.subclass ? ` (${b.subclass})` : ""), b.species_name, b.background_name].filter(Boolean).join(" · ");
}
const settingName = () => (SETTING_WORD[S.view?.setting] || S.view?.setting || "").toUpperCase();

function renderLobby(soft) {
  const v = S.view, me = mine();
  if (S.draft && v.you?.joined) {
    if (soft) return;
    return renderBuilder();
  }
  const ready = v.players.length > 0 && v.players.every(p => p.character_id);
  const waiting = v.players.filter(p => !p.character_id).map(p => p.name);
  const running = v.status === "active";
  const full = !v.you?.joined && v.players.length >= v.max_players;
  const status = running
    ? (full ? "Every seat is taken." : !v.you?.joined ? "This story is under way. Take a seat and you’ll be written in where the party stands." : "Make your character. You’ll join the party where it stands, at its level, with the story so far.")
    : !v.you?.joined ? "Take a seat to play." : !me ? "Make your character to get ready." : ready ? "Everyone is ready." : `Waiting for ${waiting.join(", ")}…`;
  const btn = v.you?.joined ? (running ? "" : `<button class="btn primary ${ready ? "" : "off"}" data-act="start">Begin the story</button>`)
    : full ? "" : `<button class="btn primary" data-act="join">Take a seat as ${esc(S.me.name)}</button>`;
  app.innerHTML = `
  <div class="lobby">
    <div class="lobby-scroll">
      <div class="lobby-art">${coverArt(S.route)}<button class="round" data-act="home" aria-label="Back home">${ICON.back}</button></div>
      <div class="lobby-body">
        <div><span class="mono-k">${running ? "Under way" : "A new story"}${settingName() ? " · " + esc(settingName()) : ""}</span><h1 class="title-l">${esc(v.title)}</h1><p class="pitch">${esc(v.pitch || v.premise)}</p></div>
        <section><span class="mono-k">Around the table</span><div class="seats" id="seats">${seatsHTML()}</div></section>
        ${v.you?.joined ? `<section><span class="mono-k">Your character</span>${me ? meCardHTML(me) : conceptHTML()}</section>` : ""}
      </div>
    </div>
    <div class="foot-bar"><div><span class="status">${esc(status)}</span>
      ${btn}
    </div></div>
  </div>`;
}

function seatsHTML() {
  const v = S.view;
  return Array.from({ length: v.max_players }, (_, i) => v.players[i]).map(p => {
    if (!p) return `<div class="seat open"><span class="face">+</span><b>Open seat</b><span>share the play code</span></div>`;
    const c = v.party.find(x => x.id === p.character_id), you = p.id === S.me?.player_id;
    return `<div class="seat ${c ? "ready" : ""} ${you ? "you" : ""}">${c ? face(c) : `<span class="face">${initial(p.name)}</span>`}
      <b>${esc(p.name)}</b><span>${c ? `${esc(firstName(c.name))}, ${esc(roleName(c).toLowerCase())}` : "making a character…"}</span></div>`;
  }).join("");
}

function meCardHTML(c) {
  return `<div class="me-card">${face(c)}<div>
    <span class="name-l">${esc(c.name)}</span>
    <span class="sub-line">${esc(roleLine(c))} · ${esc(c.pronouns || "they/them")}</span>
    ${c.concept ? `<span class="concept">${esc(c.concept)}</span>` : ""}
    <button class="link ink" data-act="edit" style="margin-top:4px;align-self:flex-start">Change something</button></div></div>`;
}

const CONCEPTS = [
  "a disgraced professor of folklore who knows too much",
  "a tired private eye with a flask and a debt to the wrong people",
  "a sharp-eyed tracker who reads a room like a trail",
  "a dock worker with fast fists and a soft heart",
];
function conceptHTML() {
  const pick = CONCEPTS[hash(S.route) % CONCEPTS.length];
  return `<textarea class="under mid" id="concept" rows="2" placeholder="Describe them in a sentence: ${esc(pick)}"></textarea>
    <div class="row"><button class="btn primary" data-act="draft">Draft my character</button><button class="link" data-act="build">or build them step by step</button></div>`;
}

// ---- character builder
//
// The 5e build, named for the campaign's era: a pulp Wizard is an
// Occultist with the same rules. The server checks every step through
// /characters/preview; the builder only arranges the choices.

const ORD = ["Cantrip", "1st", "2nd", "3rd", "4th", "5th"];
const SPELL_WORD = { fantasy: "Spells", gaslight: "Rites", pulp: "Rituals", modern: "Powers", future: "Psionics" };
const spellWord = () => SPELL_WORD[S.view?.setting] || "Spells";

// rules is the ruleset named for this campaign's setting, loaded once.
function rules() {
  const set = S.view?.setting || "fantasy";
  if (S.rules?.setting === set) return S.rules;
  if (S._rulesFor !== set) {
    S._rulesFor = set;
    api("GET", `/meta?setting=${encodeURIComponent(set)}`).then(m => { S.rules = m.ruleset; S.magic = m.magic; if (S.draft) renderBuilder(); else if (S.view) render(); }).catch(() => { S._rulesFor = ""; });
  }
  return null;
}
const rl = (list, k) => rules()?.[list]?.find(x => x.key === k);
const skillName = k => rules()?.skills?.find(s => s.key === k)?.name || k;
const spellOf = k => rl("spells", k);
function optName(kind, k) {
  if (kind === "skill") return skillName(k);
  if (kind === "feat" || kind === "style") return rules()?.feats?.[k]?.name || k;
  if (kind === "invocation") return rl("invocations", k)?.name || k;
  if (kind === "spell") return spellOf(k)?.name || k;
  if (kind === "list") return rl("classes", k)?.name || k;
  return ABIL_NAME[k] || k;
}
function optDesc(kind, k) {
  if (kind === "feat" || kind === "style") return rules()?.feats?.[k]?.desc || "";
  if (kind === "invocation") return rl("invocations", k)?.desc || "";
  if (kind === "spell") return spellOf(k)?.desc || "";
  if (kind === "skill") return ABIL_NAME[rules()?.skills?.find(s => s.key === k)?.ability] || "";
  return "";
}

function steps() {
  const p = S.prev, caster = p && (p.cantrips > 0 || p.prepared > 0);
  return ["Who", "Class", "Origin", "Abilities", "Skills", ...(caster ? [spellWord()] : []), "Story"];
}
const STEP_TITLE = { Who: "Who are they?", Class: "What do they do?", Origin: "Where are they from?", Abilities: "What are they good at?", Skills: "What have they learned?", Story: "Their story" };

function blankDraft() {
  return { name: "", class: "", pronouns: "they/them", concept: $("#concept")?.value.trim() || "", backstory: "",
    class_key: "", species: "", background: "", boost: {}, scores: {}, skills: [], picks: {}, spells: [] };
}

// preview asks the server what the draft comes to. With fill it also
// completes the draft with defaults (a new class, species or background).
let previewSeq = 0;
async function preview(fill) {
  const d = S.draft, n = ++previewSeq;
  if (!d || (!d.class_key && !fill)) { S.prev = null; return; }
  try {
    const r = await api("POST", `/campaigns/${S.route}/characters/preview`, { character: d, fill: !!fill });
    if (n !== previewSeq || S.draft !== d) return;
    if (fill) Object.assign(d, r.character, { name: d.name, pronouns: d.pronouns, concept: d.concept, backstory: d.backstory, class: d.class });
    S.prev = r;
    // Forget picks the build no longer offers (a feat or list switched away
    // from), so the chips and counts match what is on screen.
    const offered = Object.fromEntries((r.choices || []).map(ch => [ch.key, ch.options]));
    let stale = false;
    for (const [k, got] of Object.entries(d.picks || {})) {
      const keep = offered[k] ? got.filter(o => offered[k].includes(o)) : [];
      if (keep.length !== got.length) { stale = true; if (keep.length) d.picks[k] = keep; else delete d.picks[k]; }
    }
    if (stale) return preview();
  } catch (er) { toast(er.message); }
  if (S.draft === d) renderBuilder();
}

function chipsHTML(opts, on, attr, extra = () => "") {
  return `<div class="chips">${opts.map(o => `<button class="chip ${on(o) ? "on" : ""}" ${attr(o)}>${extra(o)}</button>`).join("")}</div>`;
}

function classStep(d) {
  const R = rules(), cl = R.classes, cur = cl.find(c => c.key === d.class_key);
  const list = cl.map(c => `<button class="roster-row ${c.key === d.class_key ? "on" : ""}" data-class="${esc(c.key)}" title="${esc(c.rules_name)} rules">
      <b>${esc(c.name)}</b><span></span></button>`).join("");
  const detail = cur ? `<div class="dossier">
      <span class="mono-k">d${cur.hit_die} hit die${cur.caster ? ` · ${esc(cur.cast_ability)} ${esc(spellWord().toLowerCase())}` : ""}</span>
      <h3>${esc(cur.name)}</h3><p>${esc(cur.desc)}</p>
      <dl><dt>Leans on</dt><dd>${cur.primary.map(a => ABIL_NAME[a]).join(" or ")}</dd>
        <dt>Saves</dt><dd>${cur.saves.map(a => ABIL_NAME[a]).join(", ")}</dd>
        <dt>Starts with</dt><dd>${esc(gearList(cur.gear).join(", "))}</dd>
        <dt>At 3rd level</dt><dd>${esc(cur.subclass)}</dd></dl>
      <ol class="ladder">${[1, 2, 3, 4, 5].map(l => `<li><span>${l}</span><div>${cur.features.filter(f => f.level === l).map(f => `<b title="${esc(f.desc)}">${esc(f.name)}</b>`).join("") || "<em>—</em>"}</div></li>`).join("")}</ol>
    </div>` : `<div class="dossier empty"><p>Pick a class to see what it does at each level. Every era plays the same rules under its own names.</p></div>`;
  return `<div class="roster"><div class="roster-list">${list}</div>${detail}</div>`;
}

function originStep(d) {
  const R = rules(), sp = R.species.find(s => s.key === d.species), bg = R.backgrounds.find(b => b.key === d.background);
  const b = d.boost || {}, two = Object.keys(b).find(k => b[k] === 2), three = bg && bg.abilities.every(a => b[a] === 1);
  return `<section><span class="mono-k">People</span>
      ${chipsHTML(R.species, s => s.key === d.species, s => `data-species="${esc(s.key)}"`, s => esc(s.name))}
      ${sp ? `<div class="traits"><p>${esc(sp.desc)}</p>${sp.traits.map(t => `<div><b>${esc(t.name)}</b><span>${esc(t.desc)}</span></div>`).join("")}</div>` : ""}</section>
    <section><span class="mono-k">Background</span>
      ${chipsHTML(R.backgrounds, x => x.key === d.background, x => `data-bg="${esc(x.key)}"`, x => esc(x.name))}
      ${bg ? `<div class="traits"><p>${esc(bg.desc)}</p>
        <div><b>Skills</b><span>${bg.skills.map(skillName).join(", ")}</span></div>
        <div><b>${esc(R.feats[bg.feat]?.name || bg.feat)}</b><span>${esc(R.feats[bg.feat]?.desc || "")}${bg.feat_list ? ` (${esc(rl("classes", bg.feat_list)?.name || bg.feat_list)} list)` : ""}</span></div></div>
      <span class="mono-k">Ability boost · ${three ? "+1 to all three" : "+2 to one, +1 to another"}</span>
      <div class="boost">${bg.abilities.map(a => `<div><span>${ABIL_NAME[a]}</span>
        <button class="${b[a] === 2 ? "on" : ""}" data-boost="${a}:2">+2</button><button class="${b[a] === 1 && !three ? "on" : ""}" data-boost="${a}:1">+1</button></div>`).join("")}
        <button class="spread ${three ? "on" : ""}" data-boost="all:1">+1 to each</button></div>` : ""}</section>`;
}

function abilityStep(d) {
  const cur = rl("classes", d.class_key), b = d.boost || {};
  return `<p class="intro">Tap two numbers to swap them. ${cur ? `A ${esc(cur.name.toLowerCase())} leans on ${cur.primary.map(a => ABIL_NAME[a]).join(" or ")}.` : ""}</p>
    <div class="abil-grid">${ABIL.map(a => {
      const base = d.scores?.[a] ?? 10, fin = base + (b[a] || 0), key = cur?.primary.includes(a), save = cur?.saves.includes(a);
      return `<div class="abil ${key ? "prof" : ""} ${S.swap === a ? "sel" : ""}">
        <button class="swap" data-swap="${a}" aria-label="${ABIL_NAME[a]} ${fin}"><span class="k">${a}${save ? " · save" : ""}</span><span class="v">${fin}</span>
        <span class="m">${signed(mod(fin))}${b[a] ? ` · ${base}+${b[a]}` : ""}</span></button></div>`;
    }).join("")}</div>`;
}

function skillStep(d) {
  const cur = rl("classes", d.class_key), bg = rl("backgrounds", d.background), p = S.prev;
  if (!cur || !p) return `<p class="intro">Pick a class first.</p>`;
  const fixed = new Set([...(bg?.skills || [])]);
  for (const s of p.skills || []) if (!d.skills.includes(s) && !Object.values(d.picks || {}).flat().includes(s)) fixed.add(s);
  const opts = cur.skill_choice.filter(s => !fixed.has(s));
  let out = `<section><span class="mono-k">${esc(cur.name)} skills · ${d.skills.length} of ${cur.skill_count}</span>
    ${chipsHTML(opts, s => d.skills.includes(s), s => `data-cskill="${esc(s)}"`, s => esc(skillName(s)))}
    ${fixed.size ? `<p class="small-note">Already trained: ${[...fixed].map(skillName).join(", ")}.</p>` : ""}</section>`;
  out += choicesHTML(p.choices || [], d.picks || {}, "dpick");
  return out;
}

// choicesHTML lays out build choices (a fighting style, expertise, a feat)
// as chip groups. attr is the data attribute the clicks use.
function choicesHTML(choices, picks, attr) {
  return choices.map(ch => {
    const got = picks[ch.key] || [];
    const tally = o => got.filter(x => x === o).length;
    return `<section><span class="mono-k">${esc(ch.label)} · ${got.length} of ${ch.count}</span>
      <div class="chips">${ch.options.map(o => `<button class="chip ${tally(o) ? "on" : ""}" data-${attr}="${esc(ch.key)}|${esc(o)}" title="${esc(optDesc(ch.kind, o))}">${esc(optName(ch.kind, o))}${tally(o) > 1 ? ` ×${tally(o)}` : ""}</button>`).join("")}</div>
      ${got.filter((o, i) => got.indexOf(o) === i).map(o => optDesc(ch.kind, o) && ch.kind !== "skill" ? `<p class="small-note"><b>${esc(optName(ch.kind, o))}.</b> ${esc(optDesc(ch.kind, o))}</p>` : "").join("")}</section>`;
  }).join("");
}

// spellRows lists spells to prepare, cantrips and leveled apart.
function spellRows(options, chosen, max, attr) {
  const groups = [[0, max.cantrips], [1, max.prepared]];
  return groups.filter(([, n]) => n > 0).map(([lv, n]) => {
    const list = options.map(spellOf).filter(s => s && (lv === 0 ? s.level === 0 : s.level > 0));
    const have = chosen.filter(k => list.some(s => s.key === k)).length;
    return `<section><span class="mono-k">${lv === 0 ? "Cantrips · at will" : "Prepared · uses a slot"} · ${have} of ${n}</span>
      <div class="spell-list">${list.map(s => `<button class="spell-row ${chosen.includes(s.key) ? "on" : ""}" data-${attr}="${esc(s.key)}" title="${esc(s.rules_name)}">
        <span class="dot"></span><span class="nm"><b>${esc(s.name)}</b><small>${s.level ? ORD[s.level] + " · " : ""}${esc(s.school)} · ${esc(s.range)}${s.conc ? " · concentration" : ""}</small><span>${esc(s.desc)}</span></span></button>`).join("")}</div></section>`;
  }).join("");
}

function spellStep(d) {
  const p = S.prev;
  return `<div class="row"><p class="intro" style="flex:1">Cantrips are free to cast. Prepared ${spellWord().toLowerCase()} spend a slot; you can change them after a long rest.</p><button class="btn" data-act="spells-default">Pick for me</button></div>
    ${spellRows(p.spell_options || [], d.spells || [], p, "dspell")}`;
}

// gearList folds repeats: two daggers read "Switchblade ×2".
function gearList(gear) {
  const n = new Map();
  for (const g of gear) n.set(g, (n.get(g) || 0) + 1);
  return [...n].map(([g, k]) => k > 1 ? `${g} ×${k}` : g);
}

function storyStep(d) {
  const p = S.prev, cur = rl("classes", d.class_key), sp = rl("species", d.species), bg = rl("backgrounds", d.background);
  const sc = p?.scores || {};
  return `<label class="field"><span>In a sentence</span><textarea class="under text" id="d-concept" rows="2">${esc(d.concept)}</textarea></label>
    <label class="field"><span>Backstory · optional</span><textarea class="under text" id="d-back" rows="3" placeholder="Where they come from, what they want, who they owe.">${esc(d.backstory)}</textarea></label>
    <div class="summary"><span class="face">${initial(d.name)}</span><div>
      <span class="nm">${esc(d.name || "Unnamed")}</span>
      <span class="sub-line">${esc(d.class || cur?.name || "No class yet")} · ${esc(sp?.name || "")} ${esc(bg?.name ? "· " + bg.name : "")} · ${esc(d.pronouns || "they/them")}</span>
      ${p ? `<span class="stats">${p.max_hp} HP · ${ABIL.map(a => `${a} ${sc[a] ?? "–"}`).join("  ")}</span>
      <span class="stats">${(p.skills || []).map(skillName).join(" · ")}</span>
      ${(d.spells || []).length ? `<span class="stats">${d.spells.map(k => spellOf(k)?.name || k).join(" · ")}</span>` : ""}
      ${cur ? `<span class="stats">${esc(gearList(cur.gear).join(" · "))}</span>` : ""}` : ""}
    </div></div>
    ${p?.problems?.length ? `<ul class="problems">${p.problems.map(x => `<li>${esc(x.message || x)}</li>`).join("")}</ul>` : ""}`;
}

function renderBuilder() {
  const d = S.draft, v = S.view, R = rules(), st = steps();
  S.step = Math.min(S.step, st.length - 1);
  const i = S.step, name = st[i];
  let body;
  if (!R) body = `<p class="intro"><span class="spinner"></span> Opening the rulebook…</p>`;
  else if (name === "Who") body = `<div class="grid-who">
      <label class="field"><span>Name</span><input class="under" id="d-name" value="${esc(d.name)}" maxlength="40"></label>
      <label class="field"><span>Pronouns</span><input class="under" id="d-pron" value="${esc(d.pronouns || "they/them")}" maxlength="20"></label></div>
    <label class="field"><span>What the story calls them · optional</span><input class="under" id="d-class" value="${esc(d.class)}" placeholder="${esc(rl("classes", d.class_key)?.name || "Private eye, hedge witch, medic…")}" maxlength="40">
      <span class="help">Your class sets the rules; this is just the title on the sheet.</span></label>`;
  else if (name === "Class") body = classStep(d);
  else if (!d.class_key) body = `<p class="intro">Pick a class first.</p>`;
  else if (name === "Origin") body = originStep(d);
  else if (name === "Abilities") body = abilityStep(d);
  else if (name === "Skills") body = skillStep(d);
  else if (name === "Story") body = storyStep(d);
  else body = S.prev ? spellStep(d) : "";
  const last = i === st.length - 1;
  app.innerHTML = `
  <div class="builder ${name === "Class" ? "wide" : ""}">
    <div class="builder-head"><div>
      <div class="top"><button class="x" data-act="cancel-draft" aria-label="Stop making a character">${ICON.close}</button><span class="mono-k">Your character · ${esc(v.title)}${settingName() ? " · " + esc(settingName()) : ""}</span></div>
      <div class="tabs">${st.map((s, n) => `<button class="${n === i ? "on" : ""}" data-step="${n}">${esc(s)}</button>`).join("")}</div>
    </div></div>
    <div class="builder-body"><div><h2>${esc(STEP_TITLE[name] || `Their ${name.toLowerCase()}`)}</h2>${body}<div class="error" id="d-err"></div></div></div>
    <div class="builder-foot"><div>
      <button class="back" data-act="${i === 0 ? "cancel-draft" : "step-back"}">${i === 0 ? "Cancel" : "Back"}</button>
      <button class="btn primary" data-act="${last ? "save-char" : "step-next"}">${last ? "Save character" : "Next"}</button>
    </div></div>
  </div>`;
}

function readEditor() {
  const d = S.draft;
  if (!d) return d;
  const val = id => $(id)?.value;
  if ($("#d-name")) { d.name = val("#d-name"); d.pronouns = val("#d-pron"); d.class = val("#d-class").trim(); }
  if ($("#d-back")) { d.backstory = val("#d-back"); d.concept = val("#d-concept"); }
  return d;
}

// toggle adds or removes o from list, keeping at most n (the oldest goes).
function toggle(list, o, n) {
  const i = list.indexOf(o);
  if (i >= 0) list.splice(i, 1);
  else { list.push(o); while (list.length > n) list.shift(); }
  return list;
}

// builderClick handles the builder's own buttons; true when it did.
function builderClick(ds) {
  const d = S.draft;
  if (!d) return false;
  if (ds.class) {
    readEditor();
    if (d.class_key === ds.class) return true;
    const c = rl("classes", ds.class);
    Object.assign(d, { class_key: ds.class, scores: { ...c.suggested_scores }, skills: [], picks: {}, spells: [] });
    preview(true); renderBuilder(); return true;
  }
  if (ds.species) { d.species = ds.species; d.picks = {}; preview(true); return true; }
  if (ds.bg) { d.background = ds.bg; d.boost = {}; d.picks = {}; preview(true); return true; }
  if (ds.boost) {
    const bg = rl("backgrounds", d.background), [a, n] = ds.boost.split(":"), b = d.boost || {};
    const two = Object.keys(b).find(k => b[k] === 2), one = Object.keys(b).find(k => b[k] === 1);
    const other = not => bg.abilities.find(x => x !== not && x !== a) || bg.abilities.find(x => x !== a);
    if (a === "all") d.boost = Object.fromEntries(bg.abilities.map(x => [x, 1]));
    else if (n === "2") { const o = one && one !== a ? one : other(two); d.boost = { [a]: 2, [o]: 1 }; }
    else { const t = two && two !== a ? two : other(one); d.boost = { [t]: 2, [a]: 1 }; }
    preview(); renderBuilder(); return true;
  }
  if (ds.cskill) {
    const c = rl("classes", d.class_key);
    toggle(d.skills, ds.cskill, c.skill_count);
    preview(); renderBuilder(); return true;
  }
  if (ds.dpick) {
    const [key, o] = ds.dpick.split("|"), ch = S.prev?.choices?.find(x => x.key === key);
    if (!ch) return true;
    d.picks = d.picks || {};
    const got = d.picks[key] || (d.picks[key] = []);
    if (ch.repeat && got.length < ch.count) got.push(o); else toggle(got, o, ch.count);
    preview(); renderBuilder(); return true;
  }
  if (ds.dspell) {
    const s = spellOf(ds.dspell), p = S.prev, sp = d.spells || (d.spells = []);
    const same = sp.filter(k => (spellOf(k)?.level === 0) === (s.level === 0));
    if (sp.includes(s.key)) sp.splice(sp.indexOf(s.key), 1);
    else { if (same.length >= (s.level === 0 ? p.cantrips : p.prepared)) sp.splice(sp.indexOf(same[0]), 1); sp.push(s.key); }
    preview(); renderBuilder(); return true;
  }
  return false;
}

// ---- play

// The play screen is a lamplit record. On a wide screen: a spine of
// sections on the far left (with the Tide as a waterline), the scene as an
// engraved plate with who is present, and the record itself. On a phone:
// the plate as a band over the record and the sections as a line of words
// along the bottom. Sections open as a folio laid over the scene.
const NAV = [["story", "The Story", "Story"], ["scene", "Scene", "Scene"], ["sheet", "Dossier", "Dossier"], ["journal", "Casebook", "Casebook"], ["world", "The World", "World"]];
const navItems = () => stageMode() ? NAV.filter(([k]) => k !== "scene") : NAV;

function renderPlay() {
  const stage = stageMode();
  app.innerHTML = `
  <div class="play ${stage ? "stage" : ""}" id="play">
    ${stage ? `<nav class="spine" id="spine" aria-label="Sections"></nav><aside class="plate-col" id="plate" aria-label="The scene"></aside>` : ""}
    <div class="storycol">
      <div class="${stage ? "rhead" : `banner ${S.collapsed ? "collapsed" : ""}`}" id="top"></div>
      <div class="feedwrap"><div class="feed-scroll" id="scroll"><div class="feed" id="feed"></div></div><div class="die-layer" id="die"></div></div>
      <footer class="dock" id="dock"></footer>
    </div>
    ${stage ? "" : `<nav class="ledger" id="ledger" aria-label="Sections"></nav>`}
    <div id="dimmer"></div>
    <aside class="panel" id="panel" aria-label="Panel"></aside>
    <div class="vignette" aria-hidden="true"></div>
    <div id="summons"></div>
  </div>`;
  $("#scroll").addEventListener("scroll", onFeedScroll, { passive: true });
  renderChrome();
  renderFeed(true);
  renderDie();
}

function renderChrome() {
  const play = $("#play");
  if (!play) return;
  // Strain darkens the edges of everything; a flooding Tide does too.
  const st = mine()?.strain?.value || 0, tide = S.view.world?.tide || 0;
  play.style.setProperty("--dark", Math.min(1, st / 6 * .8 + (tide > 66 ? .25 : tide > 33 ? .1 : 0)).toFixed(2));
  play.classList.toggle("unquiet", st >= 4);
  renderTop(); renderDock(); renderPanel(); renderRails(); renderNav(); renderSummons();
}

function sceneLine(upper) {
  const v = S.view, me = mine(), loc = v.location || {};
  const zone = loc.zones?.find(z => z.id === (me || v.party[0])?.zone_id)?.name;
  const when = v.scene ? `Day ${v.scene.day}, ${v.scene.time}` : "";
  const s = [zone || loc.kind, when].filter(Boolean).join(" · ");
  return upper ? s.toUpperCase() : s;
}

// flags are the few things that hang over the whole record: a fight, the
// world calling, how badly your nerve is holding.
function flagsHTML(one) {
  const v = S.view, enc = v.encounter, ev = v.world?.event, st = mine()?.strain;
  const out = [];
  if (enc) out.push(`<span class="flag red">Fight · round ${enc.round}</span>`);
  if (ev && v.status === "active" && ev.status !== "settled") out.push(`<button class="flag red" data-act="panel" data-k="world">${ev.status === "engaged" ? "The world comes due" : `The world calls · ${untilText(ev.deadline)}`}</button>`);
  if (st?.value) out.push(`<span class="flag ${st.value >= 4 ? "red" : ""}" title="Strain ${st.value} of ${st.max}">Strain ${ROMAN(st.value)}</span>`);
  return (one ? out.slice(0, 1) : out).join("");
}

function renderTop() {
  const el = $("#top");
  if (!el) return;
  const v = S.view, loc = v.location || {};
  const tide = v.world ? `<span class="tideline" role="img" aria-label="The Tide stands at ${v.world.tide}"><i style="width:${Math.max(1, Math.min(100, v.world.tide))}%"></i></span>` : "";
  if (!stageMode()) {
    el.innerHTML = `${placeArt(loc.id)}<div class="shade"></div>${tide}
      <div class="banner-in">
        <div class="bar">
          <button class="back-l" data-act="home" aria-label="Back to the archive">${ICON.back}</button>
          <span class="t"><b class="when-c">${esc(loc.name)}</b><span>${esc(v.world ? `Week ${v.world.week} · Tide ${v.world.tide}` : v.title)}</span></span>
          <span class="flags">${flagsHTML(true)}</span>
        </div>
        <div class="place-t"><span class="ty-k">${esc(sceneLine(true))}</span><b>${esc(loc.name)}</b></div>
      </div>`;
    return;
  }
  el.innerHTML = `<span class="rh-t">${esc(v.title)}</span><span class="rh-w">${esc(v.world ? `Week ${v.world.week} · ${v.world.date || ""}` : sceneLine())}</span><span class="grow"></span>${flagsHTML()}`;
}

// renderNav draws the sections: the spine on a wide screen, the line of
// words along the bottom of a phone.
function renderNav() {
  const cur = S.panel === "talk" ? "scene" : S.panel || "story";
  const items = navItems().map(([k, l, short]) => `<button class="${cur === k ? "on" : ""}" data-act="nav" data-k="${k}" ${cur === k ? `aria-current="page"` : ""}>${stageMode() ? l : short}</button>`).join("");
  const sp = $("#spine");
  if (sp) {
    const wv = S.view.world;
    sp.innerHTML = `<button class="spine-mark" data-act="home" aria-label="Back to the archive" title="Back to the archive">${EYE}</button>
      <div class="spine-nav">${items}</div>
      ${wv ? `<div class="gauge" role="img" aria-label="The Tide stands at ${wv.tide} of 100, ${esc(wv.band || "")}" title="The Tide ${wv.tide} · ${esc(wv.band || "")}">
        <span class="gauge-tube"><i style="height:${Math.max(1, Math.min(100, wv.tide))}%"></i><em style="bottom:33%"></em><em style="bottom:66%"></em></span>
        <span class="gauge-k">Tide</span><b>${wv.tide}</b></div>` : ""}`;
  }
  const lg = $("#ledger");
  if (lg) lg.innerHTML = items;
}

// ---- the plate (wide layout): the place as an engraving, who is present
// and the lay of the scene, so nothing a player checks mid-turn hides.

function renderRails() {
  const el = $("#plate");
  if (!el) return;
  if (panelBusy()) { J.stale = true; return; }
  const v = S.view, loc = v.location || {}, enc = v.encounter;
  const keep = el.querySelector(".plate-scroll")?.scrollTop || 0;
  el.innerHTML = `<div class="plate-scroll">
    <figure class="plate"><div class="plate-in">${placeArt(loc.id)}<div class="vig"></div>
      <figcaption><span class="ty-k">${esc(sceneLine(true))}</span><b>${esc(loc.name)}</b></figcaption></div></figure>
    <div class="plate-body">
      <section class="present"><p class="sh">Present</p>
        ${v.party.map(c => pcardHTML(c)).join("")}
        ${enc ? enc.enemies.map(foeRowHTML).join("") : ""}
        ${enc ? chargeHTML(enc, mine()) : ""}
      </section>
      ${sceneHTML(true)}
    </div></div>`;
  el.querySelector(".plate-scroll").scrollTop = keep;
}

function pcardHTML(c) {
  const v = S.view, acting = v.active?.character_id === c.id;
  const conds = (c.conditions || []).filter(k => k.key !== "acting");
  return `<button class="pcard ${acting ? "acting" : ""} ${isMe(c.id) ? "you" : ""} ${c.down ? "down" : ""}" data-sheet="${esc(c.id)}" title="Open ${esc(firstName(c.name))}'s dossier">
    ${face(c, "cameo")}<span class="pc-b"><b>${esc(String(c.name).replace(TITLES, "") || c.name)}${isMe(c.id) ? `<em class="ty-k">You</em>` : acting ? `<em class="ty-k">Acting</em>` : ""}</b>
      ${tally(c.hp, c.max_hp, c.name)}
      <span class="pc-k">${esc(c.class)} · ${c.hp}/${c.max_hp} HP · AC ${c.ac}${conds.length ? ` · ${conds.map(k => `<span class="cond ${esc(k.key)}" title="${esc(k.note || "")}">${esc(k.label.toLowerCase())}</span>`).join(", ")}` : ""}</span>
    </span></button>`;
}

// wound words say how a foe looks rather than its numbers.
const woundWord = e => e.hp <= 0 ? "down" : e.hp / e.max_hp > .75 ? "unhurt" : e.hp / e.max_hp > .4 ? "wounded" : e.hp / e.max_hp > .15 ? "badly hurt" : "near death";
function foeRowHTML(e, i) {
  const yours = !!S.view.you?.your_turn && S.view.budget?.action !== 0 && e.hp > 0;
  return `<button class="foe-row ${e.hp > 0 ? "" : "dead"} ${e.boss ? "boss" : ""}" ${e.hp > 0 ? `data-attack="${esc(e.id)}"` : "disabled"} title="${yours ? "Strike" : esc(e.display)}">
    <span class="lozenge" aria-hidden="true"><span>${ROMAN(i + 1)}</span></span>
    <span class="pc-b"><b>${esc(e.display)}</b>
      <span class="pc-k"><em class="${e.hp / e.max_hp <= .4 ? "red" : ""}">${woundWord(e)}</em> · ${Math.max(0, e.hp)}/${e.max_hp}${zoneName(e.zone_id) ? ` · ${esc(zoneName(e.zone_id))}` : ""}${e.hp > 0 && e.conditions?.length ? ` · ${e.conditions.map(k => esc(k.label.toLowerCase())).join(", ")}` : ""}</span>
      ${e.boss && e.hp > 0 ? `<span class="pc-k">Phase ${e.boss.phase}/${e.boss.phases} · ${esc(e.boss.phase_name)} · <span title="Legendary resistance">${"◆".repeat(e.boss.resist)}${"◇".repeat(Math.max(0, 3 - e.boss.resist))}</span></span>` : ""}
    </span>${yours ? `<span class="strike">Strike</span>` : ""}</button>`;
}

// groupsHTML shows the other groups in the shared world, with a way to go
// off alone or to join a group standing in the same place.
function groupsHTML() {
  const v = S.view, gs = v.groups || [];
  if (v.status !== "active") return "";
  const mineG = gs.find(g => g.mine), others = gs.filter(g => !g.mine);
  const canSolo = mineG && mineG.members.length > 1 && !v.encounter;
  if (!others.length && !canSolo) return "";
  return `<section class="groups"><p class="sh">Elsewhere in the world</p>
    ${others.map(g => `<div class="grp ${g.here ? "here" : ""}">
      <span class="t"><b>${esc(list(g.members.map(m => firstName(m.name))))}</b>
        <span>${g.here ? "here with you" : esc(g.location || "somewhere")}${g.fighting ? " · in a fight" : ""}</span></span>
      ${g.here && !v.encounter ? `<button class="btn grp-join" data-joingrp="${esc(g.id)}">${g.fighting ? "Join the fight" : "Join up"}</button>` : ""}
    </div>`).join("") || `<span class="empty-line">Everyone is together.</span>`}
    ${canSolo ? `<button class="link grp-solo" data-act="solo">Go off on your own</button><span class="hint-line">You keep your own scene and turns in the same world. Meet your friends somewhere to join up again.</span>` : ""}
  </section>`;
}

// downtimeHTML: at a safe place in a long campaign, one thing to do before
// the world's next turn (each morning).
function downtimeHTML() {
  const v = S.view, you = v.you || {}, kinds = v.world?.downtime || [];
  if (!kinds.length || (!you.can_downtime && !you.downtime)) return "";
  return `<section class="downtime"><p class="sh">Before morning</p>
    <span class="hint-line">Pick one thing to do overnight. It happens when the world takes its turn, around 5am Eastern.</span>
    ${kinds.map(k => `<button class="row-line ${you.downtime === k.key ? "on" : ""}" data-downtime="${you.downtime === k.key ? "" : esc(k.key)}" ${you.can_downtime || you.downtime === k.key ? "" : "disabled"}>
      <span class="nm">${esc(k.name)}<small>${esc(k.desc)}</small></span><span class="verb">${you.downtime === k.key ? "Planned · undo" : "Plan"}</span></button>`).join("")}
  </section>`;
}

async function setDowntime(kind, t) {
  if (t) t.disabled = true;
  try {
    const r = await api("POST", `/campaigns/${S.route}/me/downtime`, { kind });
    S.view = r.campaign;
    renderChrome();
  } catch (e) { toast(e.message); if (t) t.disabled = false; }
}

async function groupMove(kind, body, t) {
  if (t) t.disabled = true;
  try {
    const r = await api("POST", `/campaigns/${S.route}/me/${kind}`, body);
    S.view = r.campaign;
    S.journal = null; S.map = null;
    renderChrome(); renderFeed(true);
  } catch (e) { toast(e.message); if (t) t.disabled = false; }
}

// refreshJournal redraws the journal wherever it is showing.
function refreshJournal() { renderPanel(); renderRails(); }

const hpPct = c => Math.max(0, Math.min(100, Math.round(100 * c.hp / (c.max_hp || 1))));
const hpline = c => `<span class="hpline"><i class="${c.hp / c.max_hp <= .5 ? "low" : ""}" style="width:${hpPct(c)}%"></i></span>`;

// The banner folds away once the story is scrolled, and opens again only at
// the top of a feed long enough to scroll (so it never flickers).
let stick = true;
function onFeedScroll(e) {
  const el = e.target, top = el.scrollTop;
  stick = el.scrollHeight - top - el.clientHeight < 80;
  el.classList.toggle("scrolled", top > 0);
  const b = $(".banner");
  if (!b) return;
  if (top > 24 && !S.collapsed) { S.collapsed = true; b.classList.add("collapsed"); }
  else if (top < 4 && S.collapsed && el.scrollHeight - el.clientHeight > 150) { S.collapsed = false; b.classList.remove("collapsed"); }
}

// ---- feed

function toBottom() { const s = $("#scroll"); if (s) s.scrollTop = s.scrollHeight; }

function renderFeed(force) {
  const feed = $("#feed");
  if (!feed) { if (S.view?.status === "lobby") renderLobby(true); return; }
  const s = $("#scroll");
  if (!force && s) stick = s.scrollHeight - s.scrollTop - s.clientHeight < 80;
  const items = feedItems(currentEvents(S.events));
  // The dock already says "Your turn", so a marker that nothing follows yet is noise.
  const pend = pendingHTML();
  if (!pend && items.at(-1)?.k === "turn" && items.at(-1).you) items.pop();
  const lastNarr = items.map(it => it.k === "narr").lastIndexOf(true);
  // What a player says sits in the margin beside what came of it: an entry
  // runs from one action to the next break in the story.
  let html = "", inEntry = false, drop = true;
  const close = () => { if (inEntry) { html += "</div></div>"; inEntry = false; } };
  items.forEach((it, i) => {
    if (it.k === "act") { close(); html += `<div class="entry"><aside class="margin">${actHTML(it.c, it.you, it.text)}</aside><div class="body">`; inEntry = true; return; }
    if (it.k === "place" || it.k === "turn" || it.k === "wevent" || it.k === "gazette" || it.k === "dm") close();
    if (it.k === "place") drop = true;
    if (it.k === "narr" && drop) { it.first = true; drop = false; }
    html += itemHTML(it, i === lastNarr);
  });
  close();
  feed.innerHTML = html + dispatchHTML() + callHTML() + pend + suggestHTML();
  if (force || stick) requestAnimationFrame(toBottom);
}

// currentEvents drops character sheets a player later replaced in the
// lobby, so each seat joins the party once.
function currentEvents(evs) {
  const last = {};
  evs.forEach((e, i) => { if (e.type === "character_created") last[e.data?.player_id] = i; });
  return evs.filter((e, i) => e.type !== "character_created" || last[e.data?.player_id] === i);
}

// eachRoll pairs every d20 roll with the damage it caused.
function eachRoll(d) {
  const out = [];
  for (const r of (d.steps || []).flatMap(s => s.rolls || [])) {
    if (r.kind === "encounter") continue;
    const last = out[out.length - 1];
    if (r.kind === "damage" && last?.d20?.kind === "attack" && !last.dmg) { last.dmg = r; continue; }
    out.push(r.kind === "damage" || r.kind === "heal" ? { d20: null, dmg: r } : { d20: r, foe: !!r.foe });
  }
  return out;
}

// feedItems turns events into what the story shows: places, whose turn it
// is, what players did, the dice, the narration and quest news.
function feedItems(evs) {
  const out = [], party = S.view?.party || [];
  let lastTurn = "", joins = [], chapter = 0;
  evs = myEvents(evs);
  const flush = () => { if (joins.length) out.push({ k: "sys", text: `${list(joins)} ${joins.length > 1 ? "join" : "joins"} the party` }); joins = []; };
  for (const e of evs) {
    const d = e.data || {};
    if (e.type === "character_created") { joins.push(`${d.name} the ${d.class}`); continue; }
    flush();
    switch (e.type) {
      case "location_changed": out.push({ k: "place", name: d.name, n: ++chapter, when: d.day ? `Day ${d.day}${d.time ? `, ${d.time}` : ""}` : "" }); break;
      case "downtime_done": out.push({ k: "sys", text: d.text + (d.roll ? ` (rolled ${d.roll.total} against ${d.roll.target})` : "") }); break;
      case "world_event":
        if (d.phase === "answered") out.push({ k: "sys", text: `${isMe(d.character_id) ? "You answer" : `${d.character} answers`} the call: ${d.title}.` });
        else out.push({ k: "wevent", ...d });
        break;
      case "world_tick": {
        // days in a row with nothing between them show only the latest paper
        const it = { k: "gazette", g: d.gazette, tide: d.tide, band: d.band };
        if (out.length && out[out.length - 1].k === "gazette") out[out.length - 1] = it; else out.push(it);
        break;
      }
      case "group_split": out.push({ k: "sys", text: `${isMe(d.character_id) ? "You go" : `${d.character} goes`} off alone${d.location ? ` from ${d.location}` : ""}.` }); break;
      case "group_joined": out.push({ k: "sys", text: `${d.joining} ${d.joining?.includes(",") ? "join" : "joins"} up with ${d.with}.` }); break;
      case "turn_started": {
        // Mark the turn only when it passes to someone else.
        if (d.character_id === lastTurn) break;
        lastTurn = d.character_id;
        if (party.length > 1 || out.length === 0) out.push({ k: "turn", you: isMe(d.character_id), name: d.name });
        break;
      }
      case "player_action": {
        const c = party.find(p => p.id === d.character_id) || { name: d.character };
        out.push({ k: "act", you: isMe(d.character_id), c, text: d.text });
        break;
      }
      case "action_resolved": {
        const who = isMe(d.character_id) ? "You" : actorName(d.character_id);
        const rolls = eachRoll(d);
        const changes = (d.steps || []).flatMap(s => s.changes || []).concat(d.world_changes || []);
        rolls.forEach((g, i) => out.push(g.foe
          ? rollItem(`${e.id}.${i}`, g.d20.by || "", g, true, [], g.d20.by || "")
          : rollItem(`${e.id}.${i}`, who, g, false, i === rolls.length - 1 ? changes : [], actorName(d.character_id))));
        if (!rolls.length) changes.forEach(t => out.push({ k: "sys", text: t }));
        (d.steps || []).forEach(s => { if (s.error) out.push({ k: "sys", bad: true, text: s.error }); });
        (d.skipped || []).forEach(s => out.push({ k: "sys", text: `Not this turn: ${s.error}` }));
        break;
      }
      case "narration":
        if (d.skip && !d.text) break;
        if (d.text) out.push({ k: "narr", text: d.text, quick: d.quick, opening: d.opening });
        break;
      case "enemy_turns":
        (d.turns || []).forEach((t, i) => {
          (t.before || []).forEach(c => out.push({ k: "sys", cond: true, text: c }));
          (t.reactions || []).forEach((re, j) => {
            const a = re.attack || {};
            const g = { d20: { ...a.roll, kind: "attack" }, dmg: a.damage ? { ...a.damage, kind: "damage" } : null };
            out.push(rollItem(`${e.id}.r${i}.${j}`, isMe(re.character_id) ? "You" : re.character, g, false, [re.text], re.character));
          });
          if (t.kind === "stunned") return;
          if (t.kind === "telegraph") return out.push({ k: "warn", text: t.text });
          if (t.kind === "signature") {
            out.push({ k: "warn", hit: true, text: t.text });
            (t.saves || []).forEach((s, j) => out.push(rollItem(`${e.id}.g${i}.${j}`, s.by, { d20: s }, false, [], s.by)));
            return (t.after || []).forEach(c => out.push({ k: "sys", cond: true, text: c }));
          }
          if (!t.attack?.roll) return (t.after || []).forEach(c => out.push({ k: "sys", cond: true, text: c }));
          const g = { d20: { ...t.attack.roll, kind: "attack", label: t.attack.roll.label || `vs ${t.target || "the party"}` }, dmg: t.attack.damage ? { ...t.attack.damage, kind: "damage" } : null };
          out.push(rollItem(`${e.id}.e${i}`, t.enemy, g, true, t.text ? [t.text] : []));
          if (t.save) out.push(rollItem(`${e.id}.s${i}`, t.target, { d20: t.save }, false, [], t.target));
          (t.after || []).forEach(c => out.push({ k: "sys", cond: true, text: c }));
        });
        if (d.narration) out.push({ k: "narr", text: d.narration });
        break;
      // a private perception from strain: the server sends it only to the
      // player it happened to, and never says whether it was real
      case "vision": if (d.text) out.push({ k: "vision", text: d.text }); break;
      // the story so far, for a player who joined mid-story (only they get it)
      case "catch_up": if (d.catch_up) out.push({ k: "catchup", c: d.catch_up }); break;
      case "conditions": (d.changes || []).forEach(t => out.push({ k: "sys", cond: true, text: t })); break;
      case "rewound": out.push({ k: "place", name: "Rewound" }); out.push({ k: "sys", text: d.text }); break;
      case "dm_action": out.push({ k: "dm", text: d.text }); break;
      case "death_save": {
        const name = rollerName(d.roll?.label), c = party.find(p => p.name === name);
        out.push(rollItem(`${e.id}`, c && isMe(c.id) ? "You" : name, { d20: { ...d.roll, label: "Death save" } }, false, [d.line], name));
        if (d.died || d.stable || d.revived) out.push({ k: "sys", bad: !!d.died, text: d.line });
        break;
      }
      case "quest_updated": {
        const label = d.status === "completed" ? "QUEST DONE" : d.objective_id ? "QUEST" : d.status === "active" ? "NEW QUEST" : "QUEST";
        out.push({ k: "quest", label, text: d.text || d.title });
        break;
      }
      case "campaign_ended": out.push({ k: "place", name: d.status === "completed" ? "The story is complete" : "The story ends in failure" }); break;
    }
  }
  flush();
  return out;
}
// statuses are the engine conditions on a character (not derived states
// like bloodied or in cover).
const STATUS_KEYS = new Set(["poisoned", "prone", "restrained", "frightened", "blinded", "stunned"]);
const statuses = c => (c.conditions || []).filter(k => STATUS_KEYS.has(k.key)).map(k => k.label);

// the("The Shade") is "The Shade", the("Goblin") is "the Goblin".
const the = n => /^(the|a|an)\s/i.test(n) ? n : `the ${n}`;
const BASE_TITLE = document.title;

// untilText says how long until a unix time: "11h", "40m", "now".
function untilText(ts) {
  const s = ts - Date.now() / 1000;
  if (s <= 60) return "a moment";
  if (s < 3600) return `${Math.round(s / 60)}m`;
  return `${Math.round(s / 3600)}h`;
}
const list = a => a.length < 2 ? a.join("") : a.slice(0, -1).join(", ") + " and " + a[a.length - 1];

// The engine's labels start with the roller's name ("Silas Marrow WIS
// check to …"); the line already says who, so drop it from the label.
function shortLabel(label, name) {
  label = String(label || "");
  return name && label.toLowerCase().startsWith(name.toLowerCase() + " ") ? label.slice(name.length + 1) : label;
}

function rollItem(id, who, g, foe, changes, name) {
  const sig = g.d20 ? rollSig(g.d20, name || who) : "";
  if (g.d20) g = { ...g, d20: { ...g.d20, label: shortLabel(g.d20.label, name || who) } };
  return { k: "roll", id, who, foe, d20: g.d20, dmg: g.dmg, changes, sig };
}

function itemHTML(it, latest) {
  switch (it.k) {
    case "sys": return `<div class="ev-sys ${it.bad ? "bad" : ""} ${it.cond ? "cond-line" : ""}">${esc(it.text)}</div>`;
    case "place": return it.name === "Rewound" || !it.n ? `<div class="ev-place"><span class="nm">${esc(it.name)}</span></div>`
      : `<div class="ev-place"><span class="num">${ROMAN(it.n)}</span><span class="nm">${esc(it.name)}</span>${it.when ? `<span class="ty-k">${esc(it.when)}</span>` : ""}</div>`;
    case "turn": return `<div class="ev-turn ${it.you ? "you" : ""}">${it.you ? "Your turn" : `${esc(firstName(it.name))}’s turn`}</div>`;
    case "act": return actHTML(it.c, it.you, it.text);
    case "narr": return narrHTML(it, latest);
    case "vision": return visionHTML(it.text);
    case "catchup": return catchUpHTML(it.c);
    case "gazette": return it.g ? `<div class="ev-gazette">${gazetteHTML(it.g)}<span class="ty-k">The Tide ${it.tide} · ${esc(it.band || "")}</span></div>` : "";
    case "roll": return rollHTML(it);
    case "quest": return `<div class="ev-quest"><span class="stamp">${esc(it.label)}</span><span>${esc(it.text)}</span></div>`;
    case "warn": return `<div class="ev-warn ${it.hit ? "hit" : ""}"><b>${it.hit ? "Its worst" : "Something gathers"}</b><span>${esc(it.text)}</span></div>`;
    case "dm": return `<div class="ev-dm"><b>The table</b><span>${esc(it.text)}</span></div>`;
    case "wevent": return weventHTML(it);
  }
  return "";
}

function actHTML(c, you, text) {
  return `<p class="ev-act ${you ? "you" : ""}"><span class="who">${you ? "You" : esc(firstName(c?.name || ""))}</span><span class="said">“${esc(String(text || "").replace(/^["“]|["”]$/g, ""))}”</span></p>`;
}

// A vision is torn from somewhere else and pressed into your record only:
// misprinted, a little crooked, never the same angle twice.
// catchUpHTML: the story so far, left for a player who joined a story
// under way. Engine facts only, grouped like the "while you were away"
// letter.
const CATCH_HEADS = { quest: "The matter at hand", party: "Who you’re with", here: "Also here", fight: "The fight", promise: "The party’s word, still owed", fact: "What is known" };
function catchUpHTML(c) {
  const groups = [];
  for (const l of c.lines || []) {
    const g = groups.at(-1);
    if (g && g.kind === l.kind) g.lines.push(l); else groups.push({ kind: l.kind, lines: [l] });
  }
  return `<section class="dispatch catchup" aria-label="The story so far">
    <span class="seal" aria-hidden="true">${EYE}</span>
    <header><span class="ty-k">The story so far · for you</span><span class="dp-tide">Day ${esc(c.day)}, ${esc(c.time)} · ${esc(c.location)}</span></header>
    ${c.pitch ? `<p class="dp-l">${esc(c.pitch)}</p>` : ""}
    ${c.so_far ? `<div class="dp-group"><p class="dp-h">Until now</p><p class="dp-l">${esc(c.so_far)}</p></div>` : ""}
    ${groups.map(g => `<div class="dp-group"><p class="dp-h">${esc(CATCH_HEADS[g.kind] || "")}</p>
      ${g.lines.map(l => `<p class="dp-l">${esc(l.text)}</p>`).join("")}</div>`).join("")}
  </section>`;
}

function visionHTML(text) {
  const tilt = ((hash(text) % 15) - 7) / 10;
  return `<aside class="ev-vision" style="--tilt:${tilt}deg" aria-label="A vision only you see"><span class="ty-k">Only you notice</span><p>${esc(text)}</p></aside>`;
}

// Narration comes as plain text. Paragraphs split on blank lines; a
// paragraph that is only a quoted line reads as speech; the first words
// after arriving somewhere open with a capital set large.
function narrHTML(it, latest) {
  const paras = String(it.text).split(/\n\s*\n|\n/).map(p => p.trim()).filter(Boolean);
  return `<div class="ev-narr ${latest ? "latest" : ""} ${it.quick ? "quick" : ""}">${paras.map((p, i) => {
    const q = p.match(/^[“"](.+)[”"]$/s);
    if (q) return `<p class="say">“${esc(q[1])}”</p>`;
    return `<p class="${i === 0 && (it.first || it.opening) && /^[“"‘']?[A-Za-z]/.test(p) ? "first" : ""}">${esc(p)}</p>`;
  }).join("")}</div>`;
}

const TIER = {
  attack: { critical_success: "STRIKES TRUE", success: "STRIKES", failure: "MISSES", critical_failure: "FUMBLES" },
  save: { critical_success: "HOLDS", success: "HOLDS", failure: "BREAKS", critical_failure: "BREAKS" },
  check: { critical_success: "BEYOND HOPE", success: "SUCCEEDS", failure: "FAILS", critical_failure: "GOES BADLY" },
};
const okTier = t => t === "success" || t === "critical_success";

// A roll is a line in the ledger: who, what, the die and what it was set
// against, and the verdict. Tapping it opens the working.
function rollHTML(it) {
  const r = it.d20, dm = it.dmg;
  if (!r) {
    return `<div class="roll dmg ${dm.kind === "heal" ? "heal" : ""}"><div class="roll-line"><span class="who">${esc(it.who)}</span><span class="what">${esc(dm.label || dm.kind)}</span><span class="dice">${esc(dm.expr || "")}</span><span class="tier">${dm.total} ${dm.kind === "heal" ? "MENDED" : "WOUNDS"}</span></div></div>`;
  }
  if (DIE.isPending(it.sig)) return `<div class="ev-rolling">the die is cast…</div>`;
  const ok = okTier(r.tier), open = !!S.open[it.id];
  const words = TIER[r.kind === "attack" || r.kind === "save" ? r.kind : "check"];
  const parts = r.parts?.length ? r.parts : r.mod ? [{ label: "Modifier", value: r.mod }] : [];
  const against = r.against === "AC" || r.kind === "attack" ? "Armor" : "Difficulty";
  return `<div class="roll ${ok ? "ok" : "no"} ${it.foe ? "foe" : ""}">
    <button class="roll-line" data-roll="${esc(it.id)}" aria-expanded="${open}">
      ${it.who ? `<span class="who">${esc(it.who)}</span>` : ""}<span class="what">${esc(r.label)}</span>
      <span class="dice">d20 <b>${r.natural}</b> · ${r.total} against ${r.target}${r.edge ? ` <em class="edge" title="${esc(r.edge_why || "")}">${r.edge === "advantage" ? "favoured" : "hindered"}</em>` : ""}</span>
      <span class="tier">${words[r.tier] || ""}${dm ? ` · ${dm.total} WOUNDS` : ""}</span>
    </button>
    ${open ? `<div class="breakdown">
      <div><span>d20${r.naturals?.length ? ` (${esc(r.edge)}: rolled ${r.naturals.join(" and ")}${r.edge_why ? `, ${esc(r.edge_why)}` : ""})` : ""}</span><span>${r.natural}</span></div>
      ${parts.map(p => `<div><span>${esc(p.label)}</span><span>${signed(p.value)}</span></div>`).join("")}
      <div class="total"><span>Total</span><span>${r.total}</span></div>
      <div><span>${against}${r.cover ? ` (+${r.cover} cover)` : ""}</span><span>${r.target}</span></div>
      ${dm ? `<div><span>Wounds ${esc(dm.expr || "")}${dm.dice?.length ? ` [${dm.dice.join(", ")}]` : ""}${r.crit || r.tier === "critical_success" ? " · doubled" : ""}</span><span>${dm.total}</span></div>` : ""}
      ${(it.changes || []).map(c => `<span class="chg">${esc(c)}</span>`).join("")}
    </div>` : ""}
  </div>`;
}

// pendingHTML shows the action in flight: what was said, the dice once
// they have landed, and which step the game master is on.
// dispatchHTML: "while you were away", what the world did since this
// player last played, as a letter left for them: sealed until they break
// it, grouped from closest to home to furthest.
const DISPATCH_HEADS = { you: "Concerning yourself", deed: "Meanwhile, other investigators", people: "People you know", near: "Around you", far: "Further off" };
function dispatchHTML() {
  const d = S.view?.dispatch;
  if (!d || S.view.status !== "active") return "";
  const weeks = d.weeks === 1 ? "a week" : `${d.weeks} weeks`;
  const dt = d.tide - d.tide_from;
  const tide = dt > 0 ? `The Tide rose ${dt}, to ${d.tide}` : dt < 0 ? `The Tide fell ${-dt}, to ${d.tide}` : `The Tide held at ${d.tide}`;
  const seal = `<span class="seal" aria-hidden="true">${EYE}</span>`;
  if (!S.letterOpen) return `<section class="dispatch sealed" aria-label="While you were away">
    ${seal}<p class="ty-k">While you were away · ${weeks}</p><p class="lt">A letter has been left for you.</p>
    <button class="btn" data-act="open-letter">Break the seal</button></section>`;
  const groups = [];
  for (const l of d.lines || []) {
    const g = groups.at(-1);
    if (g && g.kind === l.kind) g.lines.push(l); else groups.push({ kind: l.kind, lines: [l] });
  }
  return `<section class="dispatch" aria-label="While you were away">
    ${seal}<header><span class="ty-k">While you were away · ${weeks}</span><span class="dp-tide">${esc(tide)} · ${esc(d.band)}</span></header>
    ${groups.map(g => `<div class="dp-group"><p class="dp-h">${esc(DISPATCH_HEADS[g.kind] || "")}</p>
      ${g.lines.map(l => `<p class="dp-l">${esc(l.text)}${l.where ? ` <span class="dp-where">Now at ${esc(l.where)}.</span>` : ""}</p>`).join("")}</div>`).join("")}
    <button class="btn dp-ok" data-act="seen">Take up the pen again</button>
  </section>`;
}

// World events: the engine calls everyone to one place when what the
// players did meets what a faction has been building. It arrives as a
// telegram: once over the whole screen (the summons), then as a standing
// wire at the foot of the story, and in full in the World section. When it
// is settled the record is stamped.
const THREAD_HEADS = { ally: "Comes to help", debt: "A debt comes due", grudge: "Against you", goal: "At stake", crossed: "What was done", brand: "Marked", pressure: "Unchecked", agenda: "Their plans" };
const wireText = s => esc(String(s || "").toUpperCase().replace(/\.\s*/g, " STOP ").replace(/\s+STOP\s*$/, " STOP").trim());
function weventHTML(it) {
  if (it.phase === "called") return `<section class="wire stub"><span class="ty-k">Telegram · the world calls</span><b class="wev-t">${esc(it.title)}</b><p class="wire-t">${wireText(it.call)}</p></section>`;
  if (it.phase === "begun") return `<div class="ev-place"><span class="num">✶</span><span class="nm">${esc(it.title)} · it begins</span></div>`;
  return `<section class="record-entry ${it.won ? "won" : "lost"}"><span class="stamp">${it.won ? "Won, for good" : "Lost, for good"}</span><b class="wev-t">${esc(it.title)}</b>
    ${(it.changes || []).map(c => `<p>${esc(c)}</p>`).join("")}</section>`;
}
function eventButtons(ev) {
  if (ev.can_answer) return `<button class="seal-btn" data-act="answer"><span>Answer<br>the call</span></button><span class="wev-wait">Go to ${esc(ev.site)} with whoever is with you.</span>`;
  if (ev.can_begin) return `<button class="seal-btn" data-act="begin"><span>Begin<br>it now</span></button><span class="wev-wait">or wait for the others to come.</span>`;
  return "";
}
function comingHTML(ev) {
  return `<p class="wev-coming">${ev.coming.map(m => `<span class="${m.came ? "came" : ""}">${esc(m.name)}${m.came ? " · there" : " · not yet"}</span>`).join("")}</p>`;
}
function callHTML() {
  const ev = S.view?.world?.event;
  if (!ev || S.view.status !== "active" || ev.status !== "gathering") return "";
  const mine = ev.threads.filter(t => t.mine);
  return `<section class="wire call" aria-label="The world calls">
    <header class="ty-k"><span>Telegram · ${untilText(ev.deadline)} left</span>${ev.here ? `<span class="wev-here">You are there</span>` : ""}</header>
    <p class="wire-t">${wireText(ev.call)}</p>
    <b class="wev-t">${esc(ev.title)}</b>
    ${mine.length ? mine.map(t => `<p class="wev-why">${esc(t.text)}</p>`).join("") : ""}
    ${comingHTML(ev)}
    <div class="wev-acts">${eventButtons(ev)}</div>
  </section>`;
}

// convergeSVG draws each thread as a line from the person it runs through,
// round the rim, curling in to the place at the centre. A faction's own
// pressure rises from below like water. Who has come rings the centre.
function convergeSVG(ev) {
  const W = 360, H = 230, cx = W / 2, cy = H / 2 - 4, R = 92;
  const ts = ev.threads.filter(t => t.kind !== "pressure").slice(0, 8);
  const press = ev.threads.some(t => t.kind === "pressure");
  const n = Math.max(ts.length, 1);
  const lines = ts.map((t, i) => {
    const a = -Math.PI / 2 + (i + .5) * (2 * Math.PI / n);
    const x = cx + R * 1.55 * Math.cos(a), y = cy + R * .95 * Math.sin(a);
    const sw = (i % 2 ? 1 : -1) * .8;
    const c1x = cx + R * .9 * Math.cos(a + sw), c1y = cy + R * .6 * Math.sin(a + sw);
    const label = t.npc || t.who || THREAD_HEADS[t.kind] || "";
    const anchor = Math.cos(a) > .25 ? "end" : Math.cos(a) < -.25 ? "start" : "middle";
    const ly = Math.max(14, Math.min(H - 6, y + (Math.sin(a) > .4 ? 14 : -6)));
    const lx = Math.max(4, Math.min(W - 4, x));
    return `<path class="th k-${esc(t.kind)} ${t.mine ? "mine" : ""}" d="M${x.toFixed(1)},${y.toFixed(1)} Q${c1x.toFixed(1)},${c1y.toFixed(1)} ${cx},${cy}" style="animation-delay:${(i * .2).toFixed(1)}s"/>
      <text class="nl k-${esc(t.kind)}" x="${lx.toFixed(1)}" y="${ly.toFixed(1)}" text-anchor="${anchor}">${esc(clip(label, 22))}</text>`;
  }).join("");
  const tide = press ? `<path class="tendril" d="M${cx - 30},${H} C${cx - 40},${H - 50} ${cx + 30},${cy + 50} ${cx},${cy}"/><path class="tendril" d="M${cx + 40},${H} C${cx + 50},${H - 40} ${cx - 20},${cy + 40} ${cx},${cy}"/>` : "";
  const dots = ev.coming.map((m, i) => {
    const a = -Math.PI / 2 + i * (2 * Math.PI / Math.max(1, ev.coming.length));
    return `<circle class="pc ${m.came ? "came" : ""}" cx="${(cx + 24 * Math.cos(a)).toFixed(1)}" cy="${(cy + 24 * Math.sin(a)).toFixed(1)}" r="3.5"><title>${esc(m.name)}${m.came ? " is there" : " has not come"}</title></circle>`;
  }).join("");
  return `<svg class="converge" viewBox="0 0 ${W} ${H}" role="img" aria-label="${esc(`${ts.length} threads lead to ${ev.site}`)}">
    <circle class="ring" cx="${cx}" cy="${cy}" r="${R * .7}"/><circle class="ring dash" cx="${cx}" cy="${cy}" r="${R * .38}"/>
    ${tide}${lines}<circle class="site" cx="${cx}" cy="${cy}" r="11"/><circle class="site-in" cx="${cx}" cy="${cy}" r="4"/>${dots}
    <text class="sl" x="${cx}" y="${cy + 46}" text-anchor="middle">${esc(ev.site)}</text></svg>`;
}

function eventPanel(ev) {
  const groups = [];
  for (const t of ev.threads) {
    const g = groups.find(x => x.kind === t.kind);
    if (g) g.lines.push(t); else groups.push({ kind: t.kind, lines: [t] });
  }
  const when = ev.status === "engaged" ? "Being fought now" : `${untilText(ev.deadline)} left to get there`;
  return `<section class="wire wev-panel"><header class="ty-k"><span>Telegram · ${esc(when)}</span></header>
    <p class="wire-t">${wireText(ev.call)}</p><b class="wev-t">${esc(ev.title)}</b>
    ${convergeSVG(ev)}
    ${groups.map(g => `<div class="dp-group k-${esc(g.kind)}"><p class="dp-h">${esc(THREAD_HEADS[g.kind] || g.kind)}</p>${g.lines.map(t => `<p class="dp-l ${t.mine ? "mine" : ""}">${esc(t.text)}</p>`).join("")}</div>`).join("")}
    ${comingHTML(ev)}
    <div class="wev-acts">${ev.status === "gathering" ? eventButtons(ev) : ""}</div>
  </section>`;
}

function summonsDone() {
  const ev = S.view?.world?.event;
  if (!ev) return;
  S.summonsGone = ev.id;
  try { localStorage.setItem(summonsKey(ev), "1"); } catch {}
  const el = $("#summons"); if (el) el.innerHTML = "";
}
// The summons takes the whole screen the first time a player sees a call.
const summonsKey = ev => `aidnd-summons-${S.route}-${ev.id}`;
function renderSummons() {
  const el = $("#summons");
  if (!el) return;
  const ev = S.view?.world?.event;
  let seen = true;
  if (ev && S.view.status === "active" && ev.status === "gathering" && S.view.you?.joined) { try { seen = localStorage.getItem(summonsKey(ev)) === "1"; } catch { seen = true; } }
  if (seen || S.summonsGone === ev?.id) { el.innerHTML = ""; return; }
  if (el.dataset.id === ev.id && el.innerHTML) return;
  el.dataset.id = ev.id;
  const by = THREAD_HEADS;
  const lines = ev.threads.filter(t => t.kind !== "pressure").slice(0, 3);
  el.innerHTML = `<div class="summons" role="dialog" aria-modal="true" aria-label="The world calls">
    <div class="summons-in">
      <p class="ty-k sm-k">The world calls</p>
      <article class="wire big"><header class="ty-k"><span>Telegram · Arkham office</span><span>Week ${S.view.world?.week || ""}</span></header>
        <p class="wire-t">${wireText(ev.call)}</p><b class="wev-t">${esc(ev.title)}</b></article>
      ${convergeSVG(ev)}
      ${lines.map(t => `<p class="sm-why"><span class="k-${esc(t.kind)}">${esc(by[t.kind] || t.kind)} ·</span> ${esc(t.text)}</p>`).join("")}
      <div class="sm-foot">${eventButtons(ev) || ""}<div class="sm-time"><b>${untilText(ev.deadline).replace(/h$/, "")}<small>${/h$/.test(untilText(ev.deadline)) ? " hours" : ""}</small></b><span>to get to ${esc(ev.site)}. It goes on without you if you don't.</span></div></div>
      <button class="link" data-act="summons-later">Not yet</button>
    </div></div>`;
}

function pendingHTML() {
  if (!isThinking()) return "";
  const p = S.progress, me = mine();
  const text = p?.text || S.pendingText || "";
  const who = p?.character_id ? S.view?.party?.find(c => c.id === p.character_id) : me;
  const you = !!who && isMe(who.id);
  const rolls = (p?.rolls || []).filter(r => r.kind !== "encounter" && r.kind !== "damage" && r.kind !== "heal")
    .map((r, i) => rollHTML(rollItem(`p${i}`, you ? "You" : who?.name || "", { d20: r }, false, [], who?.name || ""))).join("");
  const stage = p?.stage === "narrating" ? "Describing what happens" : p ? "Reading what you do" : "The game master is writing";
  return `<div class="pending">${text && S.sending || (text && p) ? actHTML(who, you, text) : ""}${rolls}<div><div class="dots"><i></i><i></i><i></i></div><div class="stage-cap">${stage}</div></div></div>`;
}

// isThinking: an action is in flight and its narration has not arrived.
function isThinking() {
  if (S.sending || S.progress) return true;
  for (let i = S.events.length - 1; i >= 0; i--) {
    const t = S.events[i].type;
    if (t === "narration" || t === "enemy_turns" || t === "turn_started" || t === "turn_budget" || t === "campaign_ended" || t === "action_pending" || t === "action_cancelled" || t === "death_save") return false;
    if (t === "player_action") return true;
  }
  return false;
}

// ---- suggested next steps: two lines under the story on your turn while
// the input is empty. They are guesses from the view; the engine decides.

function suggestions() {
  const v = S.view, me = mine(), loc = v.location || {}, b = v.budget || {};
  if (!v.you?.your_turn || isThinking() || (S.composer || "").trim() || !me || (v.pending || []).some(p => isMe(p.character_id))) return [];
  const out = [];
  if (v.encounter) {
    const foe = v.encounter.enemies.find(e => e.hp > 0);
    if (foe && b.action !== 0) out.push({ text: `Attack ${the(foe.display)} with your ${(me.weapon?.name || "weapon").toLowerCase()}`, send: `I attack ${the(foe.display)}`, hint: { target_id: foe.id } });
    const foeZones = new Set(v.encounter.enemies.filter(e => e.hp > 0).map(e => e.zone_id));
    const safe = (loc.zones || []).find(z => z.id !== me.zone_id && !foeZones.has(z.id));
    if (safe && b.move !== 0) out.push({ text: `Fall back to ${safe.name}`, send: `I move to ${safe.name}`, hint: { zone_id: safe.id } });
  } else if (S.talkTo) {
    const n = (loc.npcs || []).find(x => x.id === S.talkTo);
    const t = n && topics(n).find(tp => !S.asked[n.id + tp.key]);
    if (t) out.push({ text: `Ask ${n.name} ${t.ask}`, topic: t.key });
    out.push({ text: "End the conversation", act: "leave-talk" });
  } else {
    const n = (loc.npcs || []).find(x => x.alive !== false && !talkedTo(x));
    if (n) out.push({ text: `Talk to ${n.name}`, talk: n.id });
    if (!loc.explored) out.push({ text: `Search ${loc.name} for anything hidden`, send: "I search the area for hidden ways", hint: { check: { ability: "WIS", purpose: "search" } } });
    else {
      const x = (loc.exits || []).find(x => !x.explored && x.state !== "locked");
      if (x) out.push({ text: `Head for ${x.to_name}`, send: `I travel to ${x.to_name}` });
    }
  }
  return out.slice(0, 2);
}
function talkedTo(n) { return S.events.some(e => e.type === "player_action" && String(e.data?.text || "").includes(n.name)); }

function suggestHTML() {
  const sg = suggestions();
  S._sg = sg;
  return sg.length ? `<div class="suggest">${sg.map((s, i) => `<button data-sg="${i}"><i aria-hidden="true">☞</i><b>${esc(s.text)}</b></button>`).join("")}</div>` : "";
}

// ---- the tumbling die. A roll that just happened takes over the story for
// two seconds: it spins in, lands on the engine's number, and settles into
// the feed. Rolls arrive with the progress event or the resolved action,
// whichever is first; the signature keeps one roll from tumbling twice.

const DIE = {
  queue: [], cur: null, seen: new Set(), timer: 0,
  isPending(sig) { return !!sig && (this.cur?.sig === sig || this.queue.some(q => q.sig === sig)); },
};
const rollSig = (r, who) => [who, r.label, r.natural, r.total, r.target].join("|");

function queueRoll(r, who, foe) {
  if (!r || r.kind === "encounter" || r.kind === "damage" || r.kind === "heal" || r.natural == null) return;
  const sig = rollSig(r, who);
  if (DIE.seen.has(sig) || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  DIE.seen.add(sig);
  DIE.queue.push({ sig, r, name: who, who: isMe(S.view?.party?.find(c => c.name === who)?.id) ? "You" : who, foe });
  if (!DIE.cur) nextDie();
}

function nextDie() {
  DIE.cur = DIE.queue.shift() || null;
  renderDie();
  if (!DIE.cur) { renderFeed(); return; }
  const cur = DIE.cur;
  cur.phase = "tumble";
  let n = 0;
  const spin = setInterval(() => { const f = $("#die .face-n"); if (f) f.textContent = 1 + Math.floor(Math.random() * 20); if (++n > 40) clearInterval(spin); }, 70);
  setTimeout(() => { clearInterval(spin); if (DIE.cur !== cur) return; cur.phase = "land"; renderDie(); }, 1000);
  setTimeout(() => { if (DIE.cur === cur) nextDie(); }, 2300);
  renderFeed();
}

function renderDie() {
  const el = $("#die");
  if (!el) return;
  const d = DIE.cur;
  if (!d) { el.innerHTML = ""; return; }
  const r = d.r, ok = okTier(r.tier), land = d.phase === "land";
  const words = r.kind === "attack" ? (ok ? "Hit" : "Miss") : (ok ? "Success" : "Fail");
  const parts = r.parts?.length ? r.parts.map(p => signed(p.value)).join(" ") : r.mod ? signed(r.mod) : "";
  el.innerHTML = `<div class="die-over ${ok ? "ok" : "no"} ${d.foe ? "foe" : ""} ${land ? "" : "tumbling"}"><div>
    <div class="spinner-die">${D20}<span class="face-n">${land ? r.natural : 1 + Math.floor(Math.random() * 20)}</span></div>
    <div class="die-cap"><span class="ty-k">${esc(d.who)} · ${esc(shortLabel(r.label, d.name))}</span>
      ${land ? `<span class="res">${esc((TIER[r.kind === "attack" || r.kind === "save" ? r.kind : "check"][r.tier] || words).toLowerCase())}</span><span class="need">${r.total} against ${r.target}</span>` : `<span class="need">d20 ${esc(parts)} · needs ${r.target}</span>`}</div>
  </div></div>`;
}

// An engraved d20: the outline and inner facets of an icosahedron.
const D20 = `<svg class="d20" viewBox="0 0 100 100" aria-hidden="true"><path class="o" d="M50 3 91 26v48L50 97 9 74V26z"/><path d="M50 3 28 38h44zM28 38 9 26M72 38l19-12M28 38 50 76 72 38M28 38 9 74M72 38l19 36M50 76 9 74M50 76l41-2M50 76v21"/></svg>`;

// ---- dock

// chargeHTML warns of a boss's signature move: where it lands, and whether
// you're standing in it.
function chargeHTML(enc, me) {
  return enc.enemies.filter(e => e.hp > 0 && e.boss?.charging_zone).map(e => {
    const here = me && me.zone_id === e.boss.charging_zone;
    return `<div class="charge ${here ? "here" : ""}"><b>${esc(e.boss.move)}</b><span>${esc(e.display)} hits ${esc(zoneName(e.boss.charging_zone) || "your position")} on its next turn: ${esc(e.boss.move_dice)}, ${esc(e.boss.move_save)} save DC ${e.boss.move_dc} for half.${here ? " You're in it: move out, or Dodge for advantage on the save." : ""}</span></div>`;
  }).join("");
}

function renderDock() {
  const v = S.view, el = $("#dock");
  if (!el) return;
  const me = mine();
  if (v.status === "completed" || v.status === "failed") {
    el.innerHTML = `<div class="dock-in"><div class="ended"><span>${v.status === "completed" ? "The story is complete." : "The story has ended."}</span><button class="btn" data-act="home">Back home</button></div></div>`;
    return;
  }
  const myPend = (v.pending || []).find(p => isMe(p.character_id));
  const yours = !!v.you?.your_turn && !myPend, thinking = isThinking(), enc = v.encounter, free = !!v.free_roam;
  const groups = trays();
  if (S.tray && !groups[S.tray]?.items.length) S.tray = "";
  if (!yours) S.tray = "";
  const defs = enc ? [["attack", "Strike"], ["spells", spellWord()], ["abilities", "Abilities"], ["move", "Move"], ["tactics", "Tactics"], ["roll", "Roll"], ["use", "Use"], ["end", "End turn"]]
    : [["look", "Look"], ["talk", "Speak"], ["go", "Go"], ["spells", spellWord()], ["abilities", "Abilities"], ["roll", "Roll"], ["use", "Use"]];
  const acts = defs.filter(([k]) => k === "look" || k === "end" || groups[k]?.items.length)
    .map(([k, l]) => `<button class="${S.tray === k || (S.tray === "next" && S._next?.from === k) ? "on" : ""}" data-dock="${k}" ${thinking ? "disabled" : ""}>${l}</button>`).join("");
  const active = v.active, ap = active && v.players.find(p => p.id === active.player_id);
  const others = (v.pending || []).filter(p => !isMe(p.character_id));
  const idle = v.turn_started ? Date.now() / 1000 - v.turn_started : 0;
  const talkN = S.talkTo && (v.location?.npcs || []).find(n => n.id === S.talkTo);
  const kept = $("#say")?.value ?? S.composer;
  const stage = stageMode(), first = me ? firstName(me.name) : "";
  el.innerHTML = `<div class="dock-in">
    ${enc && !stage ? `${chargeHTML(enc, me)}<div class="foes">${enc.enemies.map(foeRowHTML).join("")}</div>` : ""}
    ${yours && S.tray ? trayHTML(groups[S.tray]) : ""}
    ${myPend ? rollCardHTML(myPend) : me?.dead ? deadCardHTML(me) : ""}
    ${others.length ? `<div class="turnrow"><span class="who other">${others.map(p => `${esc(firstName(p.name))} ${p.kind === "death_save" ? "is dying · death save" : "is about to roll"}`).join(" · ")}</span></div>` : ""}
    ${myPend ? "" : yours ? `<div class="turnrow"><span class="who">${free ? "Anyone may act." : `Your turn${first ? `, ${esc(first)}` : ""}.${v.deadline ? ` <em>The autopilot takes it in ${untilText(v.deadline)}.</em>` : ""}`}</span><span class="acts">${acts}</span></div>`
      : free ? `<div class="turnrow"><span class="who other">Anyone on their feet may act.</span></div>`
      : active && !others.some(p => p.character_id === active.character_id) ? `<div class="turnrow"><span class="who other">${esc(firstName(active.name))}’s turn. <em>${v.deadline ? `${untilText(v.deadline)} to play it` : `${esc(ap?.name || "")} is thinking`}</em></span>
      ${v.you?.joined && (v.deadline ? Date.now() / 1000 > v.deadline : idle > 120) ? `<span class="acts"><button data-act="skip">Skip their turn</button></span>` : ""}</div>` : ""}
    ${yours ? checkChipHTML() : ""}
    <form class="inputbar ${yours && !thinking ? "you" : ""}" id="say-form">
      ${stage ? "" : `<div class="party">${v.party.map(c => { const st = statuses(c); return `<button type="button" class="pa ${v.active?.character_id === c.id ? "acting" : ""} ${isMe(c.id) ? "you" : ""} ${st.length ? "afflicted" : ""} ${c.hp / c.max_hp <= .5 ? "low" : ""}" data-sheet="${esc(c.id)}" title="${esc(c.name)} · ${c.hp}/${c.max_hp} HP${st.length ? " · " + esc(st.join(", ")) : ""}">${face(c, "cameo")}</button>`; }).join("")}</div>`}
      ${thinking ? `<span class="status">The pen moves…</span>`
        : myPend ? `<span class="status">${myPend.kind === "death_save" ? "Roll your death save" : "Cast the die, or take it back"}</span>`
        : me?.dead ? `<span class="status">${esc(first)} has fallen</span>`
        : me?.down && free ? `<span class="status">${esc(first)} lies senseless until someone helps</span>`
        : yours ? `<label class="sr" for="say">What ${esc(first || "you")} ${first ? "does" : "do"}</label><textarea id="say" rows="1" enterkeyhint="send" placeholder="${esc(enc ? `Exactly what does ${first || "you"} do?` : talkN ? `Say something to ${talkN.name}…` : `What does ${first || "you"} do?`)}">${esc(kept)}</textarea>
          <button class="send" aria-label="Commit it to the record">${QUILL}</button>`
        : `<span class="status">${me?.dying ? "You lie dying…" : active ? `Waiting on ${esc(firstName(active.name))}…` : "Waiting…"}</span>`}
    </form></div>`;
  const ta = $("#say");
  if (ta) {
    autosize(ta);
    ta.oninput = () => { const had = !!S.composer.trim(); S.composer = ta.value; autosize(ta); if (had !== !!S.composer.trim()) refreshSuggest(); };
    ta.onkeydown = e => { if (e.key === "Enter" && !e.shiftKey && !matchMedia("(pointer: coarse)").matches) { e.preventDefault(); submit(); } };
  }
  $("#say-form").onsubmit = e => { e.preventDefault(); submit(); };
}
function autosize(ta) { ta.style.height = "auto"; ta.style.height = Math.min(ta.scrollHeight, 140) + "px"; }
function refreshSuggest() { const old = $(".suggest"); const tmp = document.createElement("div"); tmp.innerHTML = suggestHTML(); old ? old.replaceWith(...tmp.childNodes) : $("#feed")?.append(...tmp.childNodes); }
const zoneName = id => S.view?.location?.zones?.find(z => z.id === id)?.name || "";

// checkMod mirrors the engine's CheckMod so the tray can show the bonus
// before the roll; the engine still rolls and adds it.
function checkMod(me, a) { return (me.modifiers?.[a] ?? 0) + ((me.proficient || []).includes(a) && !me.modifiers ? me.proficiency_bonus || 0 : 0); }
// Skills come from the sheet the engine sends; proficient ones first.
const skillsOf = me => (me?.skills || []).slice().sort((a, b) => b.proficient - a.proficient);
const skillMod = (me, key) => (me?.skills || []).find(k => k.key === key)?.mod ?? 0;
// contestDC mirrors the engine's shove/grapple DC: 8 + STR + proficiency.
const contestDC = me => 8 + mod(me.scores?.STR ?? 10) + (me.proficiency_bonus || 2);

function trays() {
  const v = S.view, loc = v.location || {}, me = mine(), enc = !!v.encounter, b = v.budget || {};
  const g = { talk: { title: "Talk to", items: [] }, go: { title: "Go to", items: [] }, move: { title: "Move to · uses your move", items: [] }, attack: { title: "Attack", items: [] }, roll: { title: "Roll a check", items: [] }, use: { title: "Use", items: [] }, tactics: { title: "Tactics · uses your action", items: [] } };
  if (!me) return g;
  for (const n of loc.npcs || []) if (n.alive !== false) g.talk.items.push({ name: n.name, sub: [n.role, zoneName(n.zone_id)].filter(Boolean).join(" · "), talk: n.id });
  for (const z of loc.zones || []) if (z.id !== me.zone_id) {
    const danger = enc && v.encounter.enemies.some(e => e.hp > 0 && e.boss?.charging_zone === z.id);
    const it = { name: z.name, sub: danger ? "the boss's next attack lands here" : enc ? (b.move === 0 ? "no move left" : "uses your move") : "nearby", send: `I move to ${z.name}`, hint: { zone_id: z.id } };
    (enc ? g.move : g.go).items.push(it);
  }
  // "known" and "open" exits can both be walked; a locked one is worth trying.
  for (const x of loc.exits || []) {
    if (x.state === "locked") { if (!enc) g.go.items.push({ name: x.to_name, sub: `${x.label} · locked`, send: `I try to get through ${x.label}` }); continue; }
    (enc ? g.move : g.go).items.push({ name: x.to_name, sub: enc ? "flee · uses your action" : `${x.label} · ${x.explored ? "explored" : "unexplored"}`, send: enc ? `I flee toward ${x.to_name}` : `I travel to ${x.to_name}` });
  }
  if (enc) {
    const w = me.attack || {};
    g.attack.title = `Attack · ${me.weapon?.name || "unarmed"}${w.damage || me.weapon?.damage ? " " + (w.damage || me.weapon.damage) : ""}`;
    for (const e of v.encounter.enemies) if (e.hp > 0) g.attack.items.push({ name: e.display, sub: `${e.hp}/${e.max_hp} HP · ${zoneName(e.zone_id) || (e.ranged ? "ranged" : "")}`, send: `I attack ${the(e.display)}`, hint: { target_id: e.id } });
    // Teamwork and positioning (5e actions). They are sent as picked.
    const t = g.tactics.items, act = (kind, extra = {}) => ({ act: { kind, ...extra } });
    t.push({ name: "Dodge", sub: "attacks on you have disadvantage", send: "I take the Dodge action and focus on defense", hint: act("dodge") });
    t.push({ name: "Disengage", sub: "move away without opportunity attacks", send: "I disengage carefully", hint: act("disengage") });
    t.push({ name: "Hide", sub: `Stealth ${signed(skillMod(me, "stealth"))}`, send: "I try to hide", hint: act("hide") });
    t.push({ name: "Ready an attack", sub: "strike whatever comes within reach", send: "I ready an attack for the first enemy that comes close", hint: act("ready") });
    for (const c of v.party) if (!isMe(c.id) && !c.down && c.zone_id === me.zone_id) t.push({ name: `Help ${firstName(c.name)}`, sub: "advantage on their next roll", send: `I help ${c.name}`, hint: act("help", { target: c.id }) });
    for (const e of v.encounter.enemies) if (e.hp > 0 && e.zone_id === me.zone_id) {
      t.push({ name: `Knock ${the(e.display)} prone`, sub: `shove · DC ${contestDC(me)}`, send: `I shove ${the(e.display)} to the ground`, hint: act("shove", { target: e.id, method: "prone" }) });
      t.push({ name: `Shove ${the(e.display)} back`, sub: `push a zone · DC ${contestDC(me)}`, send: `I shove ${the(e.display)} away`, hint: act("shove", { target: e.id, method: "push" }) });
      t.push({ name: `Grapple ${the(e.display)}`, sub: `hold it fast · DC ${contestDC(me)}`, send: `I grab hold of ${the(e.display)}`, hint: act("grapple", { target: e.id }) });
    }
    for (const c of v.party) if (c.dying && c.zone_id === me.zone_id) g.use.items.push({ name: `Stabilize ${firstName(c.name)}`, bonus: `Medicine ${signed(skillMod(me, "medicine"))}`, send: `I try to stop ${c.name}'s bleeding`, hint: { check: { skill: "medicine", ability: "WIS", purpose: "stabilize" } } });
  }
  // Checks a player can call for. Search and locks are rolled exactly as
  // picked; an ability check keeps the ability and lets the game master set
  // the DC from what the player writes.
  const tools = (me.inventory || []).some(i => i.key === "thieves_tools");
  g.roll.items.push({ name: "Search for hidden ways", bonus: `Perception ${signed(skillMod(me, "perception"))}`, send: "I search the area for hidden ways", hint: { check: { ability: "WIS", skill: "perception", purpose: "search" } } });
  for (const x of loc.exits || []) {
    if (x.state !== "locked" || x.sealed) continue;
    g.roll.items.push({ name: `Pick the lock to ${x.to_name}`, bonus: `Sleight of Hand ${signed(skillMod(me, "sleight_of_hand") + (tools ? 2 : 0))}`, send: `I pick the lock on the way to ${x.to_name}`, hint: { check: { ability: "DEX", skill: "sleight_of_hand", purpose: "unlock", exit: x.id } } });
    g.roll.items.push({ name: `Force the way to ${x.to_name}`, bonus: `Athletics ${signed(skillMod(me, "athletics"))}`, send: `I force the way to ${x.to_name}`, hint: { check: { ability: "STR", skill: "athletics", purpose: "force", exit: x.id } } });
  }
  g.roll.abil = true;
  for (const i of loc.items || []) if (!enc || b.free !== 0) g.use.items.push({ name: `Take the ${i.name}`, sub: i.rarity ? `${i.rarity.replace("_", " ")} magic · ${i.magic}` : "on the ground", send: `I pick up ${the(i.name)}` });
  for (const i of me.inventory || []) if (i.kind === "consumable") g.use.items.push({ name: i.name, sub: effectText(i.effect), send: `I use the ${i.name}` });
  if (me.hp < me.max_hp && !enc) g.use.items.push({ name: "Rest a while", sub: "recover", send: "I rest for a while" });
  magicTrays(g, me);
  if (S.tray === "next" && S._next) g.next = S._next;
  return g;
}

// magicTrays adds the Spells and Abilities trays. A pick that needs a
// target opens a second tray of targets; the engine resolves the choice
// exactly as picked (a cast_spell or use_feature hint).
function magicTrays(g, me) {
  const v = S.view, b = me.build, enc = !!v.encounter, bud = v.budget || {};
  if (!b) return;
  const loc = v.location || {}, foes = enc ? v.encounter.enemies.filter(e => e.hp > 0) : [];
  const allies = v.party.filter(c => !c.dead || false);
  const slotLeft = lv => (b.slots || []).filter(s => s.level >= lv).reduce((n, s) => n + s.left, 0);
  const cost = s => ({ action: "action", bonus: "bonus action", free: "free", none: "" })[s] ?? s;
  g.spells = { title: `${spellWord()}${b.spell_dc ? ` · DC ${b.spell_dc} · attack ${signed(b.spell_attack)}` : ""}`, items: [] };
  for (const s of b.spells || []) {
    const foeSpell = ["attack", "save"].includes(s.kind) || ["hunters_mark", "hex", "magic_missile", "scorching_ray", "spiritual_weapon"].includes(s.key);
    if (!enc && (foeSpell || (s.area && s.kind !== "utility")) && !s.peace) continue;
    if (s.key === "divine_smite") continue; // rides on an attack
    const left = s.level ? slotLeft(s.level) : Infinity;
    const sub = [s.level ? `${ORD[s.level]} · ${left ? `${left} slot${left > 1 ? "s" : ""} left` : "no slots left"}` : "cantrip", enc ? cost(s.cast) : "", s.range !== "self" ? s.range : ""].filter(Boolean).join(" · ");
    const act = extra => ({ act: { kind: "cast_spell", spell: s.key, ...extra } });
    const it = { name: s.name, sub, off: !left, send: `I cast ${s.name}`, hint: act() };
    let targets = null;
    if (s.key === "knock") targets = (loc.exits || []).filter(x => x.state === "locked").map(x => ({ name: x.to_name, sub: x.label, send: `I cast ${s.name} on the way to ${x.to_name}`, hint: act({ target: x.id }) }));
    else if (s.key === "misty_step") targets = (loc.zones || []).filter(z => z.id !== me.zone_id).map(z => ({ name: z.name, sub: "step there", send: `I cast ${s.name} to ${z.name}`, hint: act({ zone: z.id }) }));
    else if (s.kind === "utility" || (s.area && s.range === "self") || s.key === "spirit_guardians") targets = null;
    else if (s.area) targets = [...new Set(foes.map(e => e.zone_id))].map(z => ({ name: zoneName(z) || "their position", sub: foes.filter(e => e.zone_id === z).map(e => e.display).join(", "), send: `I cast ${s.name} at ${zoneName(z) || "them"}`, hint: act({ zone: z }) }));
    else if (foeSpell) targets = foes.map(e => ({ name: e.display, sub: `${e.hp}/${e.max_hp} HP · ${zoneName(e.zone_id)}`, send: `I cast ${s.name} at ${the(e.display)}`, hint: act({ target: e.id }) }));
    else if (s.range !== "self" && !(s.targets > 1)) targets = [...allies, ...(s.key === "dispel_magic" ? foes.map(e => ({ id: e.id, name: e.display, hp: e.hp, max_hp: e.max_hp })) : [])]
      .filter(t => s.key === "revivify" ? t.dead : true)
      .map(t => ({ name: isMe(t.id) ? "Yourself" : t.name, sub: `${t.hp}/${t.max_hp} HP`, send: `I cast ${s.name} on ${isMe(t.id) ? "myself" : t.name}`, hint: act({ target: t.id }) }));
    if (targets) {
      if (!targets.length) continue;
      it.next = { title: `${s.name} · on whom?`, items: targets };
    }
    g.spells.items.push(it);
  }
  g.abilities = { title: "Abilities", items: [] };
  for (const f of b.abilities || []) {
    if (f.combat && !enc) continue;
    const uses = f.max ? `${f.left}/${f.max}` : "";
    const sub = f.ready ? [enc ? cost(f.slot) : "", uses].filter(Boolean).join(" · ") : f.why || "not ready";
    const act = extra => ({ act: { kind: "use_feature", feature: f.key, ...extra } });
    const it = { name: f.name, sub, off: !f.ready, title: f.desc, send: `I use ${f.name}`, hint: act() };
    if (f.target === "foe" && f.ready) it.next = { title: `${f.name} · on whom?`, items: foes.map(e => ({ name: e.display, sub: `${e.hp}/${e.max_hp} HP`, send: `I use ${f.name} on ${the(e.display)}`, hint: act({ target: e.id }) })) };
    if ((f.target === "ally" || f.target === "any") && f.ready) it.next = { title: `${f.name} · on whom?`, items: [...allies.map(t => ({ name: isMe(t.id) ? "Yourself" : t.name, sub: `${t.hp}/${t.max_hp} HP`, send: `I use ${f.name} on ${isMe(t.id) ? "myself" : t.name}`, hint: act({ target: t.id }) })), ...(f.target === "any" ? foes.map(e => ({ name: e.display, sub: `${e.hp}/${e.max_hp} HP`, send: `I use ${f.name} on ${the(e.display)}`, hint: act({ target: e.id }) })) : [])] };
    if (f.target === "zone" && f.ready) it.next = { title: `${f.name} · where?`, items: (loc.zones || []).map(z => ({ name: z.name, sub: foes.filter(e => e.zone_id === z.id).map(e => e.display).join(", ") || "", send: `I use ${f.name} toward ${z.name}`, hint: act({ zone: z.id }) })) };
    if (it.next && !it.next.items.length) continue;
    g.abilities.items.push(it);
  }
  // Riders on a weapon hit: a paladin's smite, a monk's stunning strike.
  if (enc && bud.action !== 0) {
    const has = k => (b.features || []).some(f => f.key === k);
    const smite = has("divine_smite") && slotLeft(1) > 0;
    const stun = has("stunning_strike") && (b.resources || []).some(r => r.key === "focus" && r.used < r.max);
    for (const e of foes) {
      if (smite) g.attack.items.push({ name: `${e.display} · with Divine Smite`, sub: "spends a slot on a hit", send: `I attack ${the(e.display)} and smite`, hint: { target_id: e.id, act: { kind: "attack", target: e.id, option: "smite" } } });
      if (stun) g.attack.items.push({ name: `${e.display} · Stunning Strike`, sub: "focus point on a hit", send: `I attack ${the(e.display)} with a stunning strike`, hint: { target_id: e.id, act: { kind: "attack", target: e.id, option: "stun" } } });
    }
  }
}

function trayHTML(t) {
  const me = mine();
  S._tray = t.items;
  return `<div class="tray"><span class="mono-k">${esc(t.title)}</span>
    ${t.items.map((o, i) => `<button class="tray-item ${o.off ? "off" : ""}" data-pick="${i}" ${o.title ? `title="${esc(o.title)}"` : ""}><b>${esc(o.name)}${o.next ? " ›" : ""}</b>${o.bonus ? `<span class="bonus">${esc(o.bonus)}</span>` : o.sub ? `<span>${esc(o.sub)}</span>` : ""}</button>`).join("")}
    ${t.abil && me ? `<span class="mono-k">Any other check · pick a skill, say what you try, the DM sets the difficulty</span>
      <div class="skill-grid">${skillsOf(me).map(k => `<button class="${k.proficient ? "prof" : ""}" data-skill="${esc(k.key)}"><small>${esc(k.name)}</small><b>${signed(k.mod)}</b></button>`).join("")}</div>` : ""}
  </div>`;
}

function checkChipHTML() {
  const c = S.hint?.check, me = mine();
  if (!c || !me) return "";
  const sk = c.skill && skillsOf(me).find(k => k.key === c.skill);
  const what = { search: "Search", unlock: "Pick the lock", force: "Force it", other: `${sk ? sk.name : ABIL_NAME[c.ability] || c.ability} check` }[c.purpose] || "Check";
  return `<div class="check-chip"><span class="hex">d20</span><b>${esc(what)}</b><span>${sk ? `${esc(sk.name)} ${signed(sk.mod)}` : `${esc(c.ability)} ${signed(checkMod(me, c.ability))}`}</span>
    <span class="grow">${c.purpose === "other" ? "say what you try" : "rolled when you send"}</span><button type="button" data-act="clear-check" aria-label="Drop the check">${ICON.close}</button></div>`;
}

// rollCardHTML is your planned turn waiting for your roll: each step with
// what it costs, the d20s it needs, and the button that rolls them.
function rollCardHTML(p) {
  const me = mine(), v = S.view;
  if (p.kind === "death_save") {
    const d = me?.death_saves || { successes: 0, failures: 0 };
    const pips = (n, cls) => [0, 1, 2].map(i => `<i class="${i < n ? cls : ""}"></i>`).join("");
    return `<div class="rollcard dying"><div class="rc-head"><span class="mono-k">You are dying</span><span class="pips"><span>Saves ${pips(d.successes, "ok")}</span><span>Fails ${pips(d.failures, "no")}</span></span></div>
      <p class="rc-note">Roll a d20: 10 or more is a success, a 1 counts twice, a 20 gets you back up. Three successes and you're stable${v.lethality === "story" ? "; in this story you can't die" : "; three failures and you die"}.</p>
      <div class="rc-acts"><button class="btn primary rc-roll" data-act="roll" ${S.rolling ? "disabled" : ""}><span class="hex">d20</span> Roll death save</button></div></div>`;
  }
  const steps = (p.preview || []).map(st => {
    const rolls = (st.rolls || []).map(r => `<span class="rc-d20"><span class="hex">d20</span>${signed(r.mod)} vs ${r.against === "AC" ? "AC" : "DC"} ${r.target}${r.edge ? ` <em class="edge ${esc(r.edge)}" title="${esc(r.edge_why || "")}">${r.edge === "advantage" ? "ADV" : "DIS"}</em>` : ""}</span>`).join("");
    const cost = st.error ? `<span class="rc-bad">${esc(st.error)}</span>` : st.slot && st.slot !== "free" && st.slot !== "none" ? `<span class="rc-slot">${esc(st.slot)}</span>` : "";
    return `<li class="${st.error ? "bad" : ""}"><b>${esc(st.label)}</b>${rolls}${cost}</li>`;
  }).join("");
  return `<div class="rollcard"><div class="rc-head"><span class="mono-k">Roll when ready</span><button class="link" data-act="auto-roll">Always roll for me</button></div>
    ${p.text ? `<p class="rc-said">“${esc(p.text)}”</p>` : ""}
    <ol class="rc-steps">${steps}</ol>
    <div class="rc-acts"><button class="back" data-act="take-back">Take it back</button><button class="btn primary rc-roll" data-act="roll" ${S.rolling ? "disabled" : ""}><span class="hex">d20</span> Roll</button></div></div>`;
}

function deadCardHTML(me) {
  return `<div class="rollcard dead"><div class="rc-head"><span class="mono-k">${esc(me.name)} has died</span></div>
    <p class="rc-note">Their story ends here. Make a new character to rejoin the party where they stand, with the same experience.</p>
    <div class="rc-acts"><button class="btn primary" data-act="new-char">Make a new character</button></div></div>`;
}

async function send(text, hint = {}) {
  if (!text || S.sending) return;
  if (!S.view?.you?.your_turn) return toast("Wait for your turn.");
  S.sending = true; S.pendingText = text; S.composer = ""; S.tray = ""; S.hint = {};
  closePanel(true);
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

function submit() {
  const ta = $("#say");
  let text = ta?.value.trim();
  if (!text) return;
  const hint = { ...S.hint };
  const n = S.talkTo && (S.view.location?.npcs || []).find(x => x.id === S.talkTo);
  // In a conversation, plain words are said to the person you are with.
  if (n && !S.view.encounter && !/^i\b/i.test(text)) { text = `I say to ${n.name}: “${text}”`; hint.target_id = n.id; }
  send(text, hint);
}

// ---- panels

// Sections open as a folio over the scene. The World holds the paper, the
// chart of known places and the table's controls.
const PANEL_TITLE = { scene: "The scene", sheet: "Dossier", journal: "Casebook", world: "The World", talk: "In conversation" };
const W = { tab: "world" };
const WTABS = [["world", "The World"], ["chart", "Chart"], ["table", "The Table"]];

function openPanel(k) {
  if (k === "map") { W.tab = "chart"; k = "world"; }
  if (k === "dm") { W.tab = "table"; k = "world"; }
  if (k === "story") return closePanel();
  if (k === "scene" && stageMode()) return closePanel();
  S.panel = k;
  if (k !== "talk") S.lastPanel = k;
  if (k === "world" && W.tab === "chart" && !S.map) loadMap();
  if (k === "journal" && !S.journal) loadJournal();
  renderPanel(); renderNav();
}
function closePanel(quiet) {
  if (!S.panel) return;
  S.panel = null;
  if (!quiet) { renderPanel(); renderNav(); }
  else { const p = $("#panel"); p?.classList.remove("open"); const d = $("#dimmer"); if (d) d.innerHTML = ""; renderNav(); }
}

function worldPanelHTML() {
  if (W.tab === "chart" && !S.map) loadMap();
  const tabs = S.view.world ? WTABS : WTABS.filter(([k]) => k !== "world");
  if (!tabs.some(([k]) => k === W.tab)) W.tab = tabs[0][0];
  const body = W.tab === "chart" ? mapHTML() : W.tab === "table" ? dmHTML() : worldTab();
  return `<div class="tabs">${tabs.map(([k, l]) => `<button class="${W.tab === k ? "on" : ""}" data-act="wtab" data-k="${k}">${l}</button>`).join("")}</div>${body}`;
}

function renderPanel() {
  const el = $("#panel");
  if (!el) return;
  if (!S.panel) { el.classList.remove("open"); $("#dimmer").innerHTML = ""; return; }
  if (panelBusy()) { J.stale = true; return; }
  if (S.panel === "scene" && stageMode()) { S.panel = null; return renderPanel(); }
  const k = S.panel;
  const n = k === "talk" && (S.view.location?.npcs || []).find(x => x.id === S.talkTo);
  if (k === "talk" && !n) { S.panel = null; return renderPanel(); }
  const scroll = el.classList.contains("open") && el.dataset.k === k ? el.scrollTop : 0;
  el.dataset.k = k;
  el.innerHTML = `<div class="panel-head"><h2>${PANEL_TITLE[k] || ""}</h2>
      <button class="close" data-act="close-panel" aria-label="Close">${ICON.close}</button></div>
    <div class="panel-body">${({ scene: () => sceneHTML(false), talk: () => talkHTML(n), sheet: () => sheetHTML(), journal: journalHTML, world: worldPanelHTML })[k]()}</div>`;
  el.classList.add("open");
  el.scrollTop = scroll;
  $("#dimmer").innerHTML = `<div class="scrim-dim" data-act="close-panel"></div>`;
}

// The panel re-renders on every game event. Hold off while the player is
// typing in it and catch up when they leave the field.
function panelBusy() {
  const a = document.activeElement;
  return !!a && !!a.closest?.("#panel, .rail") && a.matches("input, textarea");
}
document.addEventListener("focusout", () => setTimeout(() => { if (J.stale && !panelBusy()) { J.stale = false; refreshJournal(); } }, 0));

// relax pushes apart labels that overlap (boxes of w by h, in percent)
// and keeps them inside the frame. Pinned points do not move.
function relax(pts, w, h, lo = 10, hi = 90, ylo = lo, yhi = hi) {
  for (let it = 0; it < 120; it++) {
    let moved = false;
    for (let i = 0; i < pts.length; i++) for (let j = i + 1; j < pts.length; j++) {
      const a = pts[i], b = pts[j], dx = b.x - a.x, dy = b.y - a.y, ox = w - Math.abs(dx), oy = h - Math.abs(dy);
      if (ox <= 0 || oy <= 0) continue;
      moved = true;
      const share = a.pin ? [0, 1] : b.pin ? [1, 0] : [.5, .5];
      if (ox / w < oy / h) { const s = (dx >= 0 ? 1 : -1) * ox; a.x -= s * share[0]; b.x += s * share[1]; }
      else { const s = (dy >= 0 ? 1 : -1) * oy; a.y -= s * share[0]; b.y += s * share[1]; }
    }
    for (const p of pts) { p.x = Math.max(lo, Math.min(hi, p.x)); p.y = Math.max(p.ylo ?? ylo, Math.min(p.yhi ?? yhi, p.y)); }
    if (!moved) break;
  }
  return pts;
}

// scene board. Zones have no coordinates, so the board lays them out by
// distance from the first zone; ways out sit along the bottom edge.
function boardLayout(loc) {
  const zones = loc.zones || [];
  if (!zones.length) return { nodes: [], links: [] };
  const byId = Object.fromEntries(zones.map(z => [z.id, z]));
  const root = zones[0].id, depth = { [root]: 0 }, order = [root];
  for (let i = 0; i < order.length; i++) for (const a of byId[order[i]].adjacent || []) if (!(a in depth) && byId[a]) { depth[a] = depth[order[i]] + 1; order.push(a); }
  zones.forEach(z => { if (!(z.id in depth)) { depth[z.id] = 0; order.push(z.id); } });
  const cols = {};
  order.forEach(id => (cols[depth[id]] ||= []).push(id));
  const nc = Object.keys(cols).length, pos = {};
  Object.entries(cols).forEach(([c, ids]) => ids.forEach((id, r) => {
    const x = nc === 1 ? 50 : 16 + (+c) * (68 / (nc - 1));
    const y = ids.length === 1 ? 36 + ((+c) % 2) * 8 : 16 + r * (48 / (ids.length - 1));
    pos[id] = { x, y };
  }));
  const links = [];
  zones.forEach(z => (z.adjacent || []).forEach(a => { if (a > z.id && pos[a]) links.push({ a: pos[z.id], b: pos[a] }); }));
  const exits = loc.exits || [];
  const ex = exits.map((x, i) => ({ x, p: { x: exits.length === 1 ? 50 : 14 + i * (72 / (exits.length - 1)), y: 90, ylo: 78 } }));
  relax(Object.values(pos), 30, 22, 16, 84, 16, 84);
  ex.forEach(({ x, p }) => { const from = pos[x.zone_id] || pos[root]; links.push({ a: from, b: p, dash: true }); });
  return { pos, links, exits: ex };
}

function boardHTML() {
  const v = S.view, loc = v.location || {}, me = mine(), enc = v.encounter;
  const L = boardLayout(loc);
  if (!L.pos) return "";
  const toks = {};
  const add = (zid, t) => (toks[zid] ||= []).push(t);
  v.party.forEach(c => add(c.zone_id, `<span class="tok ${isMe(c.id) ? "you" : ""} ${c.down ? "down" : ""}" title="${esc(c.name)}">${initial(c.name)}</span>`));
  (loc.npcs || []).filter(n => n.alive !== false).forEach(n => add(n.zone_id, `<span class="tok npc" title="${esc(n.name)}">${initial(n.name)}</span>`));
  (enc?.enemies || []).filter(e => e.hp > 0).forEach(e => add(e.zone_id, `<span class="tok foe" title="${esc(e.display)}">${initial(e.display)}</span>`));
  const yy = y => (y * .75).toFixed(1);
  // Ways out are listed under the sketch rather than crowded along its foot.
  const exits = loc.exits || [];
  return `<div class="board">
    <svg class="links" viewBox="0 0 100 75" preserveAspectRatio="none">${L.links.filter(l => !l.dash).map(l => `<line x1="${l.a.x}" y1="${yy(l.a.y)}" x2="${l.b.x}" y2="${yy(l.b.y)}" ${l.dash ? `stroke-dasharray="1.4 1.2"` : ""}/>`).join("")}</svg>
    ${(loc.zones || []).map(z => `<button class="node ${z.id === me?.zone_id ? "mine" : ""}" style="left:${L.pos[z.id].x}%;top:${L.pos[z.id].y}%" data-zone="${esc(z.id)}" title="${esc(z.name)}${z.desc ? ": " + esc(z.desc) : ""}${z.cover ? " · cover" : ""}">
      <b>${esc(z.name)}${z.cover ? " ◆" : ""}</b><span class="toks">${(toks[z.id] || []).join("")}</span></button>`).join("")}
  </div>
  ${exits.length ? `<div class="ways"><span class="ty-k">Ways out</span>${exits.map(x => `<button class="way ${x.state === "locked" ? "locked" : ""}" data-exit="${esc(x.id)}" title="${esc(x.label)}${x.state === "locked" ? " · locked" : ""}">${esc(x.to_name)}${x.state === "locked" ? " (locked)" : ""}</button>`).join("")}</div>` : ""}`;
}

function sceneHTML(inPlate) {
  const v = S.view, loc = v.location || {};
  const npcs = (loc.npcs || []).filter(n => n.alive !== false);
  return `${loc.description ? `<p class="desc">${esc(loc.description)}</p>` : ""}
    ${loc.zones?.length ? `<section><p class="sh">The lay of it</p>${boardHTML()}<span class="hint-line">Tap a spot to move${v.encounter ? " (uses your move)" : ""}. Tap a way out to leave. ◆ means cover.</span></section>` : ""}
    <section><p class="sh">People here</p>
      ${npcs.map(n => `<button class="person" data-talk="${esc(n.id)}">
        <span class="t"><b>${esc(n.name)}</b><span>${esc([n.role, n.merchant ? "sells goods" : ""].filter(Boolean).join(", "))}${zoneName(n.zone_id) ? " · " + esc(zoneName(n.zone_id)) : ""}</span></span><span class="go">Speak</span></button>`).join("")
        || `<span class="empty-line">No one else is here.</span>`}
    </section>
    ${downtimeHTML()}
    ${groupsHTML()}
    ${loc.items?.length ? `<section><p class="sh">On the ground</p>${loc.items.map(i => `<button class="row-line" data-send="I pick up ${esc(the(i.name))}"><span class="nm">${esc(i.name)}${i.effect ? `<small>${esc(effectText(i.effect))}</small>` : ""}</span><span class="verb">Take</span></button>`).join("")}</section>` : ""}
    ${inPlate ? "" : `<section class="present"><p class="sh">Present</p>${v.party.map(c => pcardHTML(c)).join("")}</section>`}`;
}

// talk. The game has no list of topics, so these are prompts made from what
// the party knows; whatever is asked goes to the game master as words.
function topics(n) {
  const v = S.view, out = [];
  const given = (v.quests || []).filter(q => q.giver_name === n.name && q.status !== "completed");
  given.forEach(q => out.push({ key: "q" + q.id, label: q.title, ask: `about ${q.title.toLowerCase().replace(/^the /, "the ")}` }));
  const main = (v.quests || []).find(q => q.id === v.storyline && q.status === "active") || (v.quests || []).find(q => q.is_main && q.status === "active");
  if (main && !given.includes(main)) out.push({ key: "main", label: main.title, ask: `about ${main.title.replace(/^The /, "the ")}` });
  out.push({ key: "want", label: "What they want", ask: "what they want" });
  out.push({ key: "place", label: v.location?.name || "This place", ask: `about ${v.location?.name || "this place"}` });
  (v.location?.npcs || []).filter(o => o.id !== n.id && o.alive !== false).slice(0, 2).forEach(o => out.push({ key: "p" + o.id, label: o.name, ask: `about ${o.name}` }));
  return out;
}
const MOOD = ["hostile", "unfriendly", "wary", "neutral", "friendly", "warm", "devoted"];
const moodWord = d => MOOD[Math.max(0, Math.min(6, (d ?? 0) + 3))];

function talkHTML(n) {
  if (n.merchant && !S.stock[n.id]) {
    S.stock[n.id] = { loading: true };
    api("GET", `/campaigns/${S.route}/npcs/${n.id}`).then(r => { S.stock[n.id] = r; if (S.panel === "talk") renderPanel(); }).catch(() => { S.stock[n.id] = { stock: [] }; });
  }
  const wares = S.stock[n.id]?.stock || [];
  return `<div class="talk-id"><span class="face">${initial(n.name)}</span><div><b>${esc(n.name)}</b><span class="sub-line">${esc([n.role, moodWord(n.disposition), zoneName(n.zone_id)].filter(Boolean).join(" · "))}</span></div></div>
    <section><span class="mono-k">Ask about</span>
      ${topics(n).map(t => `<button class="topic ${S.asked[n.id + t.key] ? "asked" : ""}" data-topic="${esc(t.key)}"><b>${esc(t.label)}</b><span>${S.asked[n.id + t.key] ? "asked" : ""}</span></button>`).join("")}
      <span class="help" style="padding-top:4px">Or type anything below to say it to them.</span></section>
    ${n.merchant ? `<section><span class="mono-k">For sale</span>${S.stock[n.id]?.loading ? `<span class="empty-line">Looking over the stock…</span>` : wares.length ? wares.map(w => `<button class="row-line" data-send="I buy the ${esc(w.name)} from ${esc(n.name)}"><span class="nm">${esc(w.name)}${itemFacts(w) ? `<small>${esc(itemFacts(w))}</small>` : ""}</span><span class="price">${w.price ?? "?"} gp</span></button>`).join("") : `<span class="empty-line">Nothing for sale right now.</span>`}</section>` : ""}
    <button class="link" data-act="leave-talk" style="align-self:flex-start">End the conversation</button>`;
}

function openTalk(id) {
  const v = S.view, n = (v.location?.npcs || []).find(x => x.id === id);
  if (!n) return;
  if (v.encounter) return toast("Not in the middle of a fight.");
  S.talkTo = id; S.tray = "";
  openPanel("talk");
  renderDock();
  refreshSuggest();
}

function askTopic(key) {
  const n = (S.view.location?.npcs || []).find(x => x.id === S.talkTo);
  const t = n && topics(n).find(x => x.key === key);
  if (!t) return;
  S.asked[n.id + key] = true;
  send(`I ask ${n.name} ${t.ask}`, { target_id: n.id });
}

// character sheet: yours to act from, the rest of the party read-only

function effectText(e) {
  return String(e || "").split(";").filter(Boolean).map(p =>
    p.startsWith("heal:") ? `heals ${p.slice(5)}` : p === "revive" ? "revives the fallen" : p === "lockpick" ? "opens locks" : p).join(", ");
}
function itemFacts(i) {
  const f = [];
  if (i.kind === "weapon") f.push(i.to_hit != null ? `${signed(i.to_hit)} hit` : "", i.damage, i.reach);
  if (i.kind === "armor" && i.ac) f.push(`AC ${i.ac}`);
  if (i.kind === "shield" && i.ac) f.push(`+${i.ac} AC`);
  if (i.effect) f.push(effectText(i.effect));
  return f.filter(Boolean).join(" · ");
}

// The sheet is a case file: a photograph clipped to the corner, stamps for
// what has been done to them, then what follows them, what they swore,
// the typed record, and the working numbers last.
function sheetHTML() {
  const v = S.view, me = mine();
  const c = v.party.find(p => p.id === S.sheetFor) || me || v.party[0];
  if (!c) return `<p class="empty-line">No one has made a character yet.</p>`;
  const own = isMe(c.id), pl = playerOf(c);
  const xpFloor = c.level_xp || 0, xpP = c.next_level_xp ? Math.max(0, Math.min(1, (c.xp - xpFloor) / (c.next_level_xp - xpFloor))) : 1;
  const worn = new Set([c.weapon?.id, c.armor?.id].filter(Boolean));
  const items = (c.inventory || []).slice().sort((a, b) => (b.equipped || worn.has(b.id)) - (a.equipped || worn.has(a.id)));
  const atk = c.attack || (c.weapon ? { weapon: c.weapon.name, bonus: c.weapon.to_hit, damage: c.weapon.damage, reach: c.weapon.reach } : null);
  const stamps = [c.dead ? "Deceased" : c.down ? "Down" : "", ...(c.marks || []).slice(0, 1).map(m => m.label), ...(c.strain?.scars || []).slice(0, 1).map(() => "Scarred")].filter(Boolean);
  const fileNo = String(hash(c.id) % 9000 + 1000);
  return `${v.party.length > 1 ? `<div class="files" role="tablist" aria-label="Whose file">${v.party.map(p => `<button role="tab" class="${p.id === c.id ? "on" : ""}" data-sheet="${esc(p.id)}">${esc(firstName(p.name))}${isMe(p.id) ? " · you" : ""}</button>`).join("")}</div>` : ""}
    <div class="dossier-head">
      <div class="clip">${face(c, "photo")}<span class="paperclip" aria-hidden="true"></span></div>
      <div class="dh-t">
        <span class="ty-k">File No. ${fileNo}${pl && !own ? ` · kept by ${esc(pl.name)}` : own ? " · yours" : ""}</span>
        <span class="nm">${esc(c.name)}</span>
        <span class="sub-line">Level ${c.level} ${esc(roleLine(c))} · ${esc(c.pronouns || "they/them")}</span>
        ${c.zone_name ? `<span class="sub-line">Last seen at ${esc(c.zone_name)}</span>` : ""}
        ${c.conditions?.length ? `<span class="conds">${c.conditions.map(k => `<span class="cond ${esc(k.key)}" title="${esc(k.note || "")}">${esc(k.label)}</span>`).join("")}</span>` : ""}
      </div>
      ${stamps.length ? `<span class="stamp file-stamp">${esc(stamps[0])}</span>` : ""}
    </div>
    <div class="vitals">
      <div class="vt-hp"><span class="ty-k">Wounds</span>${tally(c.hp, c.max_hp, "Hit points")}<b>${c.hp}<small> of ${c.max_hp}</small></b>${c.build?.temp_hp ? `<span class="small-note">+${c.build.temp_hp} temporary</span>` : ""}</div>
      <div><span class="ty-k">Armor</span><b>${c.ac}</b></div>
      <div><span class="ty-k">Money</span><b>${c.gold}</b></div>
      <div><span class="ty-k">Experience</span><b>${c.xp}${c.next_level_xp ? `<small>/${c.next_level_xp}</small>` : ""}</b><span class="xpline"><i style="width:${xpP * 100}%"></i></span></div>
    </div>
    ${c.strain ? strainHTML(c.strain) : ""}
    ${consequencesHTML(c, own)}
    ${storyHTML(c, own)}
    <section><p class="sh">Faculties</p>
      <div class="stat6">${ABIL.map(a => {
        const p = !c.build && (c.proficient || []).includes(a);
        const sv = c.build?.saves?.[a];
        return `<div class="${p || sv?.proficient ? "prof" : ""}" title="${ABIL_NAME[a]}${p ? `, proficient (+${c.proficiency_bonus})` : ""}"><span>${ABIL_NAME[a] || a}</span><b>${signed(c.modifiers?.[a] ?? mod(c.scores[a]))}</b><small>${c.scores[a]}${sv ? ` · save ${signed(sv.mod)}${sv.proficient ? "★" : ""}` : ""}</small></div>`;
      }).join("")}</div>
      ${c.ac_parts?.length ? `<div class="eq">Armor ${c.ac_parts.map((p, n) => `${n ? (p.value < 0 ? " − " : " + ") : ""}${esc(p.label)} ${n ? Math.abs(p.value) : p.value}`).join("")} = <b>${c.ac}</b></div>` : ""}
      ${atk ? `<div class="eq">Strikes with <b>${esc(atk.weapon || "")}</b> ${atk.bonus != null ? signed(atk.bonus) + " to hit" : ""} · ${esc(atk.damage || "")} ${esc(atk.reach || "")}${atk.ability ? ` · ${esc(atk.ability)}` : ""}</div>` : ""}
      ${c.level_up ? `<p class="small-note">Level ${c.level_up.level} in ${Math.max(0, c.next_level_xp - c.xp)} XP: +${c.level_up.hp} max HP${c.level_up.proficiency_bonus > c.proficiency_bonus ? `, proficiency ${signed(c.level_up.proficiency_bonus)}` : ""}.</p>` : ""}
      ${c.death_saves && !c.dead ? `<div class="eq">Death saves <b>${c.death_saves.successes}</b> of 3 successes · <b>${c.death_saves.failures}</b> of 3 failures${c.death_saves.stable ? " · stable" : ""}</div>` : ""}
    </section>
    <section><p class="sh">Skills · notices at ${c.passive_perception ?? 10}</p>
      <div class="skills">${(c.skills || []).map(k => `<div class="${k.proficient ? "prof" : ""}" title="${esc(k.name)} (${esc(k.ability)})${k.proficient ? ", proficient" : ""}${k.expertise ? ", expertise" : ""}"><span>${esc(k.name)}${k.proficient ? " ★" : ""}${k.expertise ? "★" : ""}</span><b>${signed(k.mod)}</b></div>`).join("")}</div>
    </section>
    ${sheetBuildHTML(c, own)}
    <section><p class="sh">Carrying</p>
      ${items.map(i => {
        const on = i.equipped || worn.has(i.id);
        const verb = !own ? "" : i.kind === "consumable" ? "Use" : !on && ["weapon", "armor", "shield"].includes(i.kind) ? "Equip" : "";
        const note = [on ? (i.kind === "weapon" ? "in hand" : "worn") : "", i.attuned === false ? "not attuned: three others already work" : "", itemFacts(i)].filter(Boolean).join(" · ");
        const body = `<span class="nm">${esc(i.name)}${i.rarity ? ` <em class="rar ${esc(i.rarity)}">${esc(i.rarity.replace("_", " "))}</em>` : ""}${note ? `<small>${esc(note)}</small>` : ""}${i.magic ? `<small class="magic">${esc(i.magic)}</small>` : ""}</span>`;
        return verb ? `<button class="row-line" data-send="I ${verb.toLowerCase()} the ${esc(i.name)}">${body}<span class="verb">${verb}</span></button>` : `<div class="row-line">${body}</div>`;
      }).join("") || `<span class="empty-line">Nothing.</span>`}
    </section>
    ${c.backstory || c.concept ? `<section><p class="sh">Before all this</p><p class="story-p">${esc(c.backstory || c.concept)}</p></section>` : ""}
    ${own ? `<section><p class="sh">Dice</p><button class="row-line" data-act="auto-roll"><span class="nm">${v.you?.auto_roll ? "The game casts your dice" : "You cast your own dice"}<small>${v.you?.auto_roll ? "turns resolve as soon as you send them" : "attacks and checks wait for your roll"}</small></span><span class="verb">${v.you?.auto_roll ? "Roll myself" : "Roll for me"}</span></button></section>` : ""}`;
}

// storyHTML is a character's own story: what they want and what they
// chose, did and lived through, newest first. Players write their own
// goals; the story settles them.
const REC_WORD = { choice: "chose", deed: "did", outcome: "came of it" };
function storyHTML(c, own) {
  const goals = c.goals || [], rec = c.record || [];
  const open = goals.filter(g => g.status === "open"), past = goals.filter(g => g.status !== "open").slice(0, 4);
  const day = d => d ? `day ${d}` : "";
  const canAdd = own && !c.dead && open.length < 4;
  if (!goals.length && !rec.length && !own) return "";
  const row = e => `<div class="row-line rec k-${esc(e.kind)}"><span class="nm">${esc(e.text)}<small>${[REC_WORD[e.kind] || e.kind, day(e.day)].filter(Boolean).join(" · ")}</small></span></div>`;
  return `<section class="goals"><p class="sh">${own ? "What you swore" : `What ${esc(firstName(c.name))} swore`}</p>
      ${open.map(g => `<div class="row-line goal"><span class="nm">${esc(g.text)}<small>${g.source === "player" ? "set by you" : "the story gave this"}${g.day ? ` · day ${g.day}` : ""}</small></span>${own ? `<button class="verb" data-act="goal-drop" data-goal="${esc(g.id)}">Let go</button>` : ""}</div>`).join("")}
      ${past.map(g => `<div class="row-line goal k-${esc(g.status)}"><span class="nm">${esc(g.text)}<small>${g.status === "done" ? "reached" : g.status === "failed" ? "lost" : "let go"}${g.outcome ? ` · ${esc(g.outcome)}` : ""}${g.settled ? ` · day ${g.settled}` : ""}</small></span></div>`).join("")}
      ${!goals.length && !canAdd ? `<span class="empty-line">Nothing yet.</span>` : ""}
      ${canAdd ? `<div class="goal-add"><input class="under" id="goal-in" maxlength="140" placeholder="${open.length ? "Something else you want" : "Something you want: find my brother, clear my name"}"><button class="btn" data-act="goal-add">Add</button></div>
        <span class="small-note">The story decides when a goal is reached or lost. Reaching one is worth ${25} XP.</span>` : ""}
    </section>
    ${rec.length ? `<section class="record"><p class="sh">The record</p>
      ${rec.slice(0, 6).map(row).join("")}
      ${rec.length > 6 ? `<details class="more"><summary>${rec.length - 6} earlier</summary>${rec.slice(6).map(row).join("")}</details>` : ""}
    </section>` : ""}`;
}

// consequencesHTML is what follows a character between sessions: the
// marks they carry and the people who hold something toward them.
function consequencesHTML(c, own) {
  const marks = c.marks || [], ties = c.standing || [];
  const day = d => d ? ` · day ${d}` : "";
  const you = own ? "you" : firstName(c.name);
  const tieWord = t => t.kind === "grudge" ? "holds a grudge" : t.kind === "debt" ? `owes ${you}` : `${you} owe${own ? "" : "s"} them`;
  return `${marks.length ? `<section class="marks"><p class="sh">Marks</p>
      ${marks.map(m => `<div class="row-line cmark k-${esc(m.kind)}"><span class="nm">${esc(m.text)}<small>${esc(m.label.toLowerCase())}${day(m.day)}</small></span></div>`).join("")}
    </section>` : ""}
    ${ties.length ? `<section class="standing"><p class="sh">Who remembers ${own ? "you" : esc(firstName(c.name))}</p>
      ${ties.map(t => `<div class="row-line tie k-${esc(t.kind)}${t.alive ? "" : " gone"}"><span class="nm">${esc(t.name)}${t.alive ? "" : " (dead)"}<small><em>${esc(tieWord(t))}</em> · ${esc(t.text)}${day(t.day)}</small></span></div>`).join("")}
    </section>` : ""}`;
}

// sheetBuildHTML is the class half of a sheet: level-up picks waiting,
// limited uses, slots and spells, and features.
function sheetBuildHTML(c, own) {
  const b = c.build, v = S.view;
  if (!b) return "";
  const pips = (left, max) => `<span class="pips">${Array.from({ length: max }, (_, i) => `<i class="${i < left ? "ok" : ""}"></i>`).join("")}</span>`;
  let out = "";
  if (b.made_before_classes) out += `<p class="small-note">Made before classes, so they play as a ${esc(b.class_name.toLowerCase())}.${own && v.status === "lobby" ? " Change something to rebuild them." : ""}</p>`;
  if (own && b.pending_choices.length) {
    S._sc = S._sc || {};
    const picks = Object.fromEntries(b.pending_choices.map(ch => [ch.key, S._sc[ch.key] || [...(b.picks?.[ch.key] || [])]]));
    out += `<section class="levelup"><span class="mono-k">New level · choose</span>${choicesHTML(b.pending_choices, picks, "spick")}
      <button class="btn primary" data-act="save-choices">Keep these</button></section>`;
  }
  if (b.resources.length) out += `<section><span class="mono-k">Uses</span>${b.resources.map(r => `<div class="row-line"><span class="nm">${esc(r.name)}<small>back after a ${r.recharge} rest</small></span>${r.max > 6 ? `<b class="note">${r.max - r.used}/${r.max}</b>` : pips(r.max - r.used, r.max)}</div>`).join("")}</section>`;
  if (b.caster) {
    const sw = spellWord();
    out += `<section><span class="mono-k">${esc(sw)}${b.spell_dc ? ` · save DC ${b.spell_dc} · attack ${signed(b.spell_attack)}` : ""}</span>
      ${b.slots?.length ? `<div class="slots">${b.slots.map(s => `<div><span>${ORD[s.level]}</span>${pips(s.left, s.max)}</div>`).join("")}</div>` : ""}
      ${b.concentration ? `<p class="small-note">Concentrating on <b>${esc(b.concentration_name)}</b>. Taking damage may break it.</p>` : ""}
      ${S.prepFor === c.id && b.prepare ? `${spellRows(b.prepare.options, S._prep, b.prepare, "sspell")}
        <div class="row"><button class="back" data-act="prep-cancel">Cancel</button><button class="btn primary" data-act="prep-save">Prepare these</button></div>`
        : `${(b.spells || []).slice().sort((x, y) => x.level - y.level).map(s => `<div class="row-line"><span class="nm">${esc(s.name)}<small>${s.level ? ORD[s.level] : "cantrip"} · ${esc(s.range)}${s.conc ? " · concentration" : ""} · ${esc(s.desc)}</small></span></div>`).join("")}
        ${own && b.prepare && !v.encounter ? `<button class="link ink" data-act="prep-edit">Change prepared ${esc(sw.toLowerCase())}</button>` : ""}`}
    </section>`;
  }
  out += `<section><span class="mono-k">Features${b.feats.length ? " · feats" : ""}</span>
    ${b.features.map(f => `<details class="feat"><summary>${esc(f.name)}<small>${f.level ? `level ${f.level}` : "trait"}</small></summary><p>${esc(f.desc)}</p></details>`).join("")}
    ${b.feats.map(k => `<details class="feat"><summary>${esc(rules()?.feats?.[k]?.name || k)}<small>feat</small></summary><p>${esc(rules()?.feats?.[k]?.desc || "")}</p></details>`).join("")}
  </section>`;
  return out;
}

// sheetPick toggles a level-up pick or a spell being prepared on the sheet.
function sheetPick(ds) {
  const me = mine(), b = me?.build;
  if (!b) return;
  if (ds.spick) {
    const [key, o] = ds.spick.split("|"), ch = b.pending_choices.find(x => x.key === key);
    S._sc = S._sc || {};
    const got = S._sc[key] || (S._sc[key] = [...(b.picks?.[key] || [])]);
    if (ch.repeat && got.length < ch.count) got.push(o); else toggle(got, o, ch.count);
  } else {
    const s = spellOf(ds.sspell) || b.spells.find(x => x.key === ds.sspell), sp = S._prep;
    const same = sp.filter(k => ((spellOf(k)?.level ?? 1) === 0) === (s.level === 0));
    if (sp.includes(s.key)) sp.splice(sp.indexOf(s.key), 1);
    else { if (same.length >= (s.level === 0 ? b.prepare.cantrips : b.prepare.prepared)) sp.splice(sp.indexOf(same[0]), 1); sp.push(s.key); }
  }
  renderPanel();
}

// ---- DM console
//
// Table-side controls any seated player can use. Pages are listed in
// DM_PAGES so more can be added; each change goes through the engine and
// shows in everyone's feed.

const DM_PAGES = [["history", "Rewind"], ["fight", "Fight"], ["party", "Party"], ["items", "Give item"], ["rules", "Rules"]];

function loadHistory() {
  api("GET", `/campaigns/${S.route}/dm/history`).then(r => { S.dmHist = r.snapshots; refreshJournal(); }).catch(er => toast(er.message));
}

function dmHTML() {
  const v = S.view, page = S.dmPage || "history";
  if (!v.you?.joined) return `<p class="empty-line">Take a seat to use the table controls.</p>`;
  let body = "";
  if (page === "history") {
    if (!S.dmHist) { loadHistory(); body = `<p class="empty-line"><span class="spinner"></span></p>`; }
    else body = `<p class="small-note" style="margin:0 0 6px">Put the story back to just before a turn. Everything after it is undone for everyone: rolls, damage, loot and narration. The last 30 turns are kept.</p>
      ${S.dmHist.map(h => `<div class="row-line"><span class="nm">${esc(h.label)}<small>turn ${h.turn} · ${ago(h.created_at)}</small></span>
        ${S.dmConfirm === h.seq ? `<span class="dm-confirm"><button class="link" data-act="dm-cancel">Keep</button><button class="btn danger" data-act="dm-rewind" data-seq="${h.seq}">Rewind</button></span>`
          : `<button class="verb" data-act="dm-ask" data-seq="${h.seq}">Rewind to here</button>`}</div>`).join("") || `<p class="empty-line">No turns played yet.</p>`}`;
  } else if (page === "fight") {
    const enc = v.encounter;
    body = enc ? `<p class="small-note" style="margin:0 0 10px">${enc.enemies.filter(e => e.hp > 0).map(e => esc(e.display)).join(", ")} still stand${enc.enemies.filter(e => e.hp > 0).length === 1 ? "s" : ""}.</p>
      <button class="btn" data-act="dm" data-kind="end_fight">End the fight as won</button><p class="small-note">The foes yield or flee; the party gets the XP and loot.</p>`
      : `<p class="empty-line">No fight under way.</p>`;
  } else if (page === "party") {
    body = `<button class="btn" data-act="dm" data-kind="heal_party">Heal everyone to full</button><p class="small-note">Wakes the fallen and clears conditions outside a fight. The dead stay dead.</p>
      <div class="dm-row"><label class="field"><span>Award XP to everyone</span><input class="under" id="dm-xp" type="number" min="1" max="3000" value="100"></label><button class="btn" data-act="dm" data-kind="award_xp">Award</button></div>`;
  } else if (page === "items") {
    const M = S.magic;
    if (!M) { rules(); body = `<p class="empty-line"><span class="spinner"></span></p>`; }
    else {
      const to = v.party.find(c => c.id === S.dmTo) || mine() || v.party[0];
      S.dmTo = to?.id;
      const gear = (to?.inventory || []).filter(i => (i.kind === "weapon" || i.kind === "armor" || i.kind === "shield") && !i.rarity && i.key);
      const row = (attrs, name, rar, desc) => `<button class="row-line" ${attrs}><span class="nm">${esc(name)} <em class="rar ${esc(rar)}">${esc(rar.replace("_", " "))}</em>${desc ? `<small>${esc(desc)}</small>` : ""}</span><span class="verb">Give</span></button>`;
      body = `<span class="mono-k">To</span>${chipsHTML(v.party, c => c.id === S.dmTo, c => `data-dmto="${esc(c.id)}"`, c => esc(firstName(c.name)))}
        <section><span class="mono-k">Wondrous items and potions</span>${M.wondrous.map(w => row(`data-act="dm-give" data-item="${esc(w.key)}"`, w.name, w.rarity, w.desc)).join("")}</section>
        ${gear.length ? `<section><span class="mono-k">Enchant what ${esc(firstName(to.name))} carries</span>${gear.flatMap(g => M.templates.filter(t => t.for === (g.kind === "weapon" ? "weapon" : "armor")).map(t =>
          row(`data-act="dm-give" data-item="${esc(g.key)}" data-template="${esc(t.key)}"`, t.name.replace("%s", g.name), t.rarity, ""))).join("")}</section>` : ""}`;
    }
  } else if (page === "rules") {
    const story = v.lethality === "story";
    body = `<div class="row-line"><span class="nm">${story ? "Death is off" : "Death is on"}<small>${story ? "three failed death saves leave a character stable" : "three failed death saves kill"}</small></span>
      <button class="verb" data-act="dm" data-kind="lethality" data-value="${story ? "standard" : "story"}">${story ? "Turn death on" : "Turn death off"}</button></div>`;
  }
  return `<div class="tabs dm-tabs">${DM_PAGES.map(([k, l]) => `<button class="${k === page ? "on" : ""}" data-act="dm-page" data-k="${k}">${l}</button>`).join("")}</div><div class="dm-body">${body}</div>`;
}

async function dmAct(ds, t) {
  const body = { kind: ds.kind };
  if (ds.kind === "award_xp") body.amount = +($("#dm-xp")?.value || 0);
  if (ds.value) body.value = ds.value;
  if (ds.item) { body.kind = "give_item"; body.target = S.dmTo; body.item = ds.item; if (ds.template) body.template = ds.template; }
  t.disabled = true;
  try {
    const r = await api("POST", `/campaigns/${S.route}/dm`, body);
    toast(r.text.replace(/^.*?\(DM console\): /, ""));
  } finally { await refreshView(); refreshJournal(); }
}

// journal. Quests, people, places and clues come from the world record on
// the server; notes are the player's own and private.

const J = { tab: "quests", q: "", notes: null, notesFor: "", draft: "", editing: "", stale: false };
const JTABS = [["quests", "Quests"], ["people", "People"], ["places", "Places"], ["clues", "Clues"], ["notes", "Notes"]];

function loadJournal() {
  api("GET", `/campaigns/${S.route}/journal`).then(j => { S.journal = j; refreshJournal(); }).catch(() => {});
}
function loadNotes() {
  if (J.notesFor === S.route) return;
  J.notesFor = S.route; J.notes = null;
  if (!S.view?.you?.joined) { J.notes = []; return; }
  api("GET", `/campaigns/${S.route}/notes`).then(r => { J.notes = r.notes; refreshJournal(); }).catch(() => { J.notes = []; });
}
async function addNote(text) {
  const n = await api("POST", `/campaigns/${S.route}/notes`, { text });
  (J.notes ||= []).push(n);
  return n;
}

function journalHTML() {
  if (!S.journal) loadJournal();
  loadNotes();
  const j = S.journal;
  const find = J.tab === "quests" || J.tab === "notes" || J.tab === "world" ? "" : `<input class="find" id="j-find" type="search" placeholder="Search ${J.tab}" value="${esc(J.q)}" autocomplete="off">`;
  let body;
  if (J.tab === "world") body = worldTab();
  else if (J.tab === "quests") body = questsTab();
  else if (J.tab === "notes") body = notesTab();
  else if (!j) body = `<p class="empty-line">Opening the journal…</p>`;
  else body = ({ people: peopleTab, places: placesTab, clues: cluesTab })[J.tab](j);
  if (J.tab === "world") J.tab = "quests";
  const tabs = JTABS;
  return `<div class="tabs">${tabs.map(([k, l]) => `<button class="${J.tab === k ? "on" : ""}" data-act="jtab" data-k="${k}">${l}</button>`).join("")}</div>${find}<div id="j-body" style="display:flex;flex-direction:column;gap:22px">${body}</div>`;
}

const match = (...parts) => !J.q || parts.join(" ").toLowerCase().includes(J.q.toLowerCase());
const none = s => `<p class="empty-line">${s}</p>`;

// worldTab: the shared calendar, the Tide and this week's newspaper.
function worldTab() {
  const wv = S.view.world;
  if (!wv) return none("This story has no world clock.");
  return `<div class="world-now"><span class="mono-k">Week ${wv.week} · ${esc(wv.date)}</span>
      ${tideHTML(wv)}
    </div>
    ${wv.event ? eventPanel(wv.event) : ""}
    ${wv.gazette ? gazetteHTML(wv.gazette, wv.date) : none("The first paper comes out after the world's next turn, each morning.")}
    <span class="hint-line">The world takes a turn every morning, even when nobody plays: the week moves on, the factions push their plans and the Tide rises. Finishing quests holds it back. When what you have done collides with what a faction is building, everyone is called to one place, and what happens there lasts.</span>
    ${wv.past_events?.length ? `<div class="dp-group"><p class="dp-h">Turning points</p><ul>${wv.past_events.map(e => `<li><b>${esc(e.title)}</b> · week ${e.week} · ${e.won ? "won" : "lost"}. ${esc(e.outcome)}</li>`).join("")}</ul></div>` : ""}`;
}

function tideHTML(wv) {
  const word = { Ebb: "Ebb · the dark is held back", Slack: "Slack · things are getting worse", Flood: "Flood · the dark is winning" }[wv.band] || wv.band;
  return `<div class="tide"><div class="tide-bar"><i style="width:${Math.max(2, Math.min(100, wv.tide))}%"></i><em style="left:33%"></em><em style="left:66%"></em></div>
    <span class="tide-k"><b>The Tide ${wv.tide}</b> · ${esc(word)}</span></div>`;
}

function gazetteHTML(g, date) {
  return `<article class="gazette"><header><b>The Arkham Advertiser</b><span>${esc(date || `Week ${g.week}`)}</span></header>
    ${(g.headlines || []).map((h, i) => `<p class="${i === 0 ? "lead-h" : ""}">${esc(h)}</p>`).join("")}
    ${g.classified ? `<p class="classified"><span class="mono-k">Classified</span>${esc(g.classified)}</p>` : ""}
    ${g.weather ? `<p class="weather">${esc(g.weather)}</p>` : ""}</article>`;
}

function questsTab() {
  const qs = S.view.quests.filter(q => q.status !== "available" || q.is_main).sort((a, b) => (a.status === "completed") - (b.status === "completed") || b.is_main - a.is_main);
  if (!qs.length) return none("No quests yet. Talk to people and they will ask things of you.");
  return qs.map(q => `<div class="quest ${q.status === "completed" || q.status === "failed" ? "done-q" : ""}">
    <span class="mono-k">${q.is_main ? "Main quest" : "Side quest"}${q.giver_name ? ` · from ${esc(q.giver_name)}` : ""}${q.status !== "active" ? ` · ${esc(q.status.replace("_", " "))}` : ""}</span>
    <span class="q-title">${esc(q.title)}</span>
    ${q.summary ? `<p class="q-sum">${esc(q.summary)}</p>` : ""}
    ${q.objectives.map(o => `<div class="obj ${o.done ? "done" : ""}"><i></i><span>${esc(o.text)}</span></div>`).join("")}
    ${q.reward_gold || q.reward_xp ? `<span class="reward">Reward ${[q.reward_gold && q.reward_gold + " gold", q.reward_xp && q.reward_xp + " XP"].filter(Boolean).join(" · ")}</span>` : ""}
  </div>`).join("");
}

function peopleTab(j) {
  const ps = (j.people || []).filter(p => match(p.name, p.role, p.desc, p.location_name));
  if (!j.people?.length) return none("Nobody yet. Whoever you talk to shows up here, with what they told you and what they think of you.");
  if (!ps.length) return none(`Nobody matches “${esc(J.q)}”.`);
  return ps.map(p => `<div class="jp ${p.alive ? "" : "dead"}"><span class="face round-f">${initial(p.name)}</span><div>
    <b>${esc(p.name)}</b>
    <span class="meta">${p.here ? `<span class="here">HERE</span> · ` : ""}${esc((p.role || "").toUpperCase())}${p.role ? " · " : ""}${p.alive ? moodWord(p.disposition).toUpperCase() : "DEAD"} · ${p.met ? "SPOKE WITH AT" : "SEEN AT"} ${esc((p.location_name || "").toUpperCase())}</span>
    ${p.desc ? `<span class="note">${esc(p.desc)}</span>` : ""}
    ${p.quests?.length ? `<span class="note">Gave you: ${p.quests.map(esc).join(", ")}</span>` : ""}
    ${p.here && p.alive && !S.view.encounter ? `<button class="link ink talk" data-talk="${esc(p.id)}">Talk →</button>` : ""}
  </div></div>`).join("");
}

function placesTab(j) {
  const ps = (j.places || []).filter(p => match(p.name, p.kind, p.hint, p.description, (p.people || []).join(" ")));
  if (!j.places?.length) return none("No places yet.");
  if (!ps.length) return none(`No place matches “${esc(J.q)}”.`);
  return ps.map(p => `<div class="jpl ${p.visited ? "" : "unseen"}"><b>${esc(p.name)}</b>
    <span class="meta mono-k" style="font-size:10.5px">${p.here ? `<span style="color:var(--acc)">You are here</span> · ` : ""}${esc(p.kind || "")}${p.is_hub ? " · home" : ""}${p.danger ? ` · danger ${p.danger}` : ""}${p.visited ? "" : " · heard of"}</span>
    ${p.visited ? (p.description ? `<span class="note">${esc(p.description)}</span>` : "") : p.hint ? `<span class="note">${esc(p.hint)}</span>` : ""}
    ${p.people?.length ? `<span class="note" style="color:var(--ink3)">People: ${p.people.map(esc).join(", ")}</span>` : ""}
  </div>`).join("");
}

function cluesTab(j) {
  const cs = (j.chronicle || []).filter(c => c.kind !== "news" && match(c.text, c.kind)).slice().reverse();
  const kind = c => c.kind === "promise" ? `<span class="kind ${esc(c.status || "open")}">promise${c.status && c.status !== "open" ? " " + esc(c.status) : ""}</span>` : c.kind && c.kind !== "fact" ? `<span class="kind">${esc(c.kind)}</span>` : "";
  return `${j.summary && !J.q ? `<section><span class="mono-k">The story so far</span><p class="sofar">${esc(j.summary)}</p></section>` : ""}
    ${!j.chronicle?.length ? none("No clues yet. Search, ask, look closely. What you find is written here.") : !cs.length ? none(`No clue matches “${esc(J.q)}”.`)
      : cs.map(c => `<div class="clue"><p>${esc(c.text)}${kind(c)}</p>${S.view.you?.joined ? `<button class="jot" data-act="jot" data-text="${esc(c.text)}" title="Copy to your notes" aria-label="Copy to your notes">${ICON.plus}</button>` : ""}</div>`).join("")}`;
}

function notesTab() {
  if (!S.view.you?.joined) return none("Join this story to keep notes.");
  if (!J.notes) return none("Opening your notes…");
  const stamp = n => `Day ${n.day || 1} · ${esc(n.time || "")}`;
  return `<form class="note-new" id="n-new"><textarea id="n-text" rows="4" placeholder="Suspicions, names, plans…">${esc(J.draft)}</textarea>
      <div class="row"><span>Only you can read these.</span><button class="btn">Write it down</button></div></form>
    ${J.notes.length ? `<div class="notes">${J.notes.slice().reverse().map(n => J.editing === n.id
      ? `<form class="note" data-note="${esc(n.id)}"><span class="when">${stamp(n)}</span><textarea id="n-edit" rows="4">${esc(n.text)}</textarea>
          <div class="row"><button type="button" class="del" data-act="note-del" data-id="${esc(n.id)}">Delete</button><button type="button" class="link" data-act="note-cancel">Cancel</button><button class="btn primary">Save</button></div></form>`
      : `<button class="note" data-act="note-edit" data-id="${esc(n.id)}"><span class="when">${stamp(n)}</span><p>${esc(n.text)}</p></button>`).join("")}</div>`
      : none("No notes yet. Tap + on a clue to copy it here.")}`;
}

// map: known places laid out around where the party stands.

function loadMap() {
  if (S.mapLoading) return;
  S.mapLoading = true;
  api("GET", `/campaigns/${S.route}/map`).then(m => { S.map = m; if (S.panel === "world") renderPanel(); }).catch(() => {}).finally(() => { S.mapLoading = false; });
}

function mapLayout(m) {
  const here = m.current_id || m.hub_id, ids = m.locations.map(l => l.id);
  const adj = {};
  m.edges.forEach(e => { (adj[e.a] ||= []).push(e.b); (adj[e.b] ||= []).push(e.a); });
  const depth = { [here]: 0 }, order = [here];
  for (let i = 0; i < order.length; i++) for (const n of adj[order[i]] || []) if (!(n in depth)) { depth[n] = depth[order[i]] + 1; order.push(n); }
  ids.forEach(id => { if (!(id in depth)) { depth[id] = 3; order.push(id); } });
  const rings = {};
  order.forEach(id => (rings[depth[id]] ||= []).push(id));
  const pos = { [here]: { x: 50, y: 52, pin: true } };
  Object.entries(rings).forEach(([d, ring]) => {
    if (+d === 0) return;
    const r = Math.min(40, 20 + (+d - 1) * 13), off = (hash(here) % 360) * Math.PI / 180 + (+d) * .6;
    ring.forEach((id, i) => {
      const a = off + (i / ring.length) * Math.PI * 2;
      pos[id] = { x: Math.max(10, Math.min(90, 50 + Math.cos(a) * r)), y: Math.max(8, Math.min(92, 52 + Math.sin(a) * r)) };
    });
  });
  relax(Object.values(pos), 30, 13, 15, 85, 8, 92);
  return pos;
}

function mapHTML() {
  if (!S.map) return none("Unfolding the map…");
  const m = S.map, pos = mapLayout(m), here = m.current_id;
  return `<div class="worldmap">
      <svg viewBox="0 0 100 100" preserveAspectRatio="none">${m.edges.filter(e => pos[e.a] && pos[e.b]).map(e => `<line class="${e.state === "locked" ? "locked" : ""}" x1="${pos[e.a].x}" y1="${pos[e.a].y}" x2="${pos[e.b].x}" y2="${pos[e.b].y}"/>`).join("")}</svg>
      ${m.locations.map(l => `<button class="mnode ${l.id === here ? "here" : l.visited ? "visited" : ""}" style="left:${pos[l.id].x}%;top:${pos[l.id].y}%" ${l.id === here ? "disabled" : `data-send="I travel to ${esc(l.name)}"`} title="${esc(l.hint || l.description || l.name)}">
        <span class="d"></span><span class="lb"><b>${esc(l.name)}</b><span>${l.id === here ? "you are here" : l.visited ? "explored" : "unexplored"}</span></span></button>`).join("")}
    </div>
    <span class="hint-line">Only places you’ve seen or heard of. Tap one to travel there.</span>`;
}

// ---- events from the page

let lastLayout = `${stageMode()}|${sideMode()}`;
addEventListener("resize", () => {
  const now = `${stageMode()}|${sideMode()}`;
  if (now !== lastLayout) { lastLayout = now; if ($("#play")) renderPlay(); }
});

document.addEventListener("input", e => {
  if (e.target.id === "j-find") {
    J.q = e.target.value;
    const pos = e.target.selectionStart;
    const tmp = document.createElement("div");
    tmp.innerHTML = journalHTML();
    $("#j-body").replaceWith(tmp.querySelector("#j-body"));
    const f = $("#j-find"); f.focus(); f.setSelectionRange(pos, pos);
  }
  if (e.target.id === "n-text") J.draft = e.target.value;
});

document.addEventListener("submit", async e => {
  const f = e.target;
  if (f.id === "n-new") {
    e.preventDefault();
    const text = $("#n-text").value.trim();
    if (!text) return;
    try { await addNote(text); J.draft = ""; refreshJournal(); } catch (er) { toast(er.message); }
  } else if (f.dataset?.note) {
    e.preventDefault();
    const id = f.dataset.note, text = $("#n-edit").value.trim();
    if (!text) return;
    try {
      await api("POST", `/campaigns/${S.route}/notes/${id}`, { text });
      const n = J.notes.find(n => n.id === id); if (n) n.text = text;
      J.editing = ""; refreshJournal();
    } catch (er) { toast(er.message); }
  }
});

document.addEventListener("click", async e => {
  const t = e.target.closest("[data-act],[data-open],[data-premise],[data-saga],[data-era],[data-lethal],[data-class],[data-species],[data-bg],[data-boost],[data-cskill],[data-dpick],[data-dspell],[data-spick],[data-sspell],[data-swap],[data-step],[data-sheet],[data-dock],[data-pick],[data-check],[data-skill],[data-sg],[data-roll],[data-attack],[data-talk],[data-topic],[data-zone],[data-exit],[data-send],[data-dmto],[data-joingrp],[data-downtime]");
  if (!t || t.disabled) return;
  const ds = t.dataset, act = ds.act;
  if (ds.open) return go(`#/c/${ds.open}`);
  if (ds.saga) return startSaga(ds.saga);
  if (ds.premise) { $("#premise").value = ds.premise[0].toUpperCase() + ds.premise.slice(1); return; }
  if (ds.era != null && !act) { S.era = ds.era; $("#eras").outerHTML = erasHTML(); return; }
  if (ds.lethal) { S.lethality = ds.lethal; $("#death").outerHTML = deathHTML(); return; }
  if (ds.dmto) { S.dmTo = ds.dmto; return refreshJournal(); }
  if (builderClick(ds)) return;
  if (ds.spick || ds.sspell) return sheetPick(ds);
  if (ds.swap) {
    const d = readEditor(), a = ds.swap;
    if (!S.swap) S.swap = a;
    else if (S.swap === a) S.swap = "";
    else { [d.scores[a], d.scores[S.swap]] = [d.scores[S.swap], d.scores[a]]; S.swap = ""; preview(); }
    return renderBuilder();
  }
  if (ds.step) { readEditor(); S.step = +ds.step; return renderBuilder(); }
  if (ds.sheet) { S.sheetFor = ds.sheet; return openPanel("sheet"); }
  if (ds.dock) {
    const k = ds.dock;
    if (k === "look") return send("I look around carefully");
    if (k === "end") { S.tray = ""; return api("POST", `/campaigns/${S.route}/skip`, {}).catch(er => toast(er.message)); }
    S.tray = S.tray === k ? "" : k;
    // A taller dock shrinks the feed, so keep the latest lines in view.
    const sc = $("#scroll"), near = sc && sc.scrollHeight - sc.scrollTop - sc.clientHeight < 80;
    renderDock();
    if (near) toBottom();
    return;
  }
  if (ds.pick) {
    const o = S._tray?.[+ds.pick];
    if (!o) return;
    if (o.talk) return openTalk(o.talk);
    if (o.off) return toast(o.sub);
    if (o.next) { S._next = { ...o.next, from: S.tray }; S.tray = "next"; return renderDock(); }
    return send(o.send, o.hint);
  }
  if (ds.check) { S.hint = { check: { ability: ds.check, purpose: "other" } }; S.tray = ""; renderDock(); const ta = $("#say"); if (ta) { ta.value = S.composer = ta.value || "I try to "; ta.focus(); } return; }
  if (ds.skill) {
    const sk = skillsOf(mine()).find(k => k.key === ds.skill);
    S.hint = { check: { ability: sk?.ability || "WIS", skill: ds.skill, purpose: "other" } }; S.tray = ""; renderDock();
    const ta = $("#say"); if (ta) { ta.value = S.composer = ta.value || "I try to "; ta.focus(); } return;
  }
  if (ds.sg) {
    const s = S._sg?.[+ds.sg];
    if (!s) return;
    if (s.talk) return openTalk(s.talk);
    if (s.topic) return askTopic(s.topic);
    if (s.act === "leave-talk") { S.talkTo = ""; closePanel(); renderDock(); return renderFeed(); }
    return send(s.send, s.hint);
  }
  if (ds.roll) { S.open[ds.roll] = !S.open[ds.roll]; return renderFeed(); }
  if (ds.attack) { const f = S.view.encounter?.enemies.find(x => x.id === ds.attack); return f && send(`I attack ${the(f.display)}`, { target_id: f.id }); }
  if (ds.talk) return openTalk(ds.talk);
  if (ds.topic) return askTopic(ds.topic);
  if (ds.zone) {
    const me = mine();
    if (ds.zone === me?.zone_id) return;
    const z = S.view.location.zones.find(x => x.id === ds.zone);
    return send(`I move to ${z.name}`, { zone_id: z.id });
  }
  if (ds.exit) {
    const x = S.view.location.exits.find(x => x.id === ds.exit);
    if (!x) return;
    return send(x.state === "locked" ? `I try to get through ${x.label}` : S.view.encounter ? `I flee toward ${x.to_name}` : `I travel to ${x.to_name}`);
  }
  if (ds.send) return send(ds.send);
  if (ds.joingrp) return groupMove("join", { group: ds.joingrp }, t);
  if (ds.downtime !== undefined) return setDowntime(ds.downtime, t);
  try {
    switch (act) {
      case "solo": return groupMove("solo", {}, t);
      case "seen": return groupMove("seen", {}, t);
      case "answer": summonsDone(); return groupMove("answer", {}, t);
      case "summons-later": summonsDone(); return;
      case "open-letter": S.letterOpen = true; return renderFeed();
      case "begin": summonsDone(); return groupMove("begin", {}, t);
      case "signout": await fetch("/auth/logout", { method: "POST" }); S.me = null; leaveCampaign(); S.route = ""; location.hash = ""; return renderSignin();
      case "home": return go("#/");
      case "create": return createCampaign();
      case "join":
        t.disabled = true;
        await api("POST", `/campaigns/${S.route}/players`, { name: S.me.name });
        return refreshView();
      case "draft": {
        const el = $("#concept");
        const concept = el.value.trim() || el.placeholder.replace(/^Describe them in a sentence: /, "");
        t.disabled = true; t.innerHTML = `<span class="spinner"></span> Drafting…`;
        try {
          const r = await api("POST", `/campaigns/${S.route}/characters/draft`, { concept });
          S.draft = { ...blankDraft(), ...r.character, concept: r.character.concept || concept };
          S.prev = null; await preview(true);
          S.step = steps().length - 1;
        } finally { renderLobby(); }
        return;
      }
      case "build": S.draft = blankDraft(); S.prev = null; S.step = 0; return renderLobby();
      case "edit": {
        const c = mine();
        const spec = c.build?.spec;
        // Sheets made before classes are rebuilt from their kit.
        S.draft = spec ? { ...blankDraft(), ...structuredClone(spec) } : { ...blankDraft(), name: c.name, class: c.class, kit: c.kit, pronouns: c.pronouns, backstory: c.backstory || "", concept: c.concept || "" };
        S.prev = null; S.step = 0;
        renderLobby();
        await preview(!spec);
        return;
      }
      case "step-next":
        readEditor();
        if (steps()[S.step] === "Class" && !S.draft.class_key) return toast("Pick a class first.");
        S.step = Math.min(steps().length - 1, S.step + 1); return renderBuilder();
      case "spells-default": if (S.prev) { S.draft.spells = [...(S.prev.default_spells || [])]; preview(); } return renderBuilder();
      case "step-back": readEditor(); S.step = Math.max(0, S.step - 1); return renderBuilder();
      case "cancel-draft": S.draft = null; S.prev = null; S.swap = ""; return render();
      case "save-char": {
        const d = readEditor();
        t.disabled = true;
        try {
          await api("POST", `/campaigns/${S.route}/characters`, { character: d });
          S.draft = null;
          await refreshView();
          render();
        } catch (er) { $("#d-err").textContent = er.message; t.disabled = false; }
        return;
      }
      case "start":
        if (t.classList.contains("off")) return toast("Everyone needs a character first.");
        t.disabled = true; t.innerHTML = `<span class="spinner"></span> Setting the scene…`;
        await api("POST", `/campaigns/${S.route}/start`);
        return refreshView();
      case "menu": return S.panel ? closePanel() : openPanel(S.lastPanel || "scene");
      case "panel": return S.panel === ds.k ? closePanel() : openPanel(ds.k);
      case "nav": return ds.k === "story" || S.panel === ds.k ? closePanel() : openPanel(ds.k);
      case "wtab": W.tab = ds.k; if (ds.k === "chart" && !S.map) loadMap(); return renderPanel();
      case "close-panel": if (S.panel === "talk") S.talkTo = S.talkTo; return closePanel();
            case "leave-talk": S.talkTo = ""; closePanel(); renderDock(); return renderFeed();
      case "clear-check": S.hint = {}; return renderDock();
      case "skip": return api("POST", `/campaigns/${S.route}/skip`, {});
      case "roll":
        S.rolling = true; renderDock();
        try { await api("POST", `/campaigns/${S.route}/roll`, {}); }
        finally { S.rolling = false; await refreshView(); renderFeed(); }
        return;
      case "take-back": {
        const p = (S.view.pending || []).find(x => isMe(x.character_id));
        await api("POST", `/campaigns/${S.route}/roll/cancel`, {});
        S.composer = p?.text || "";
        await refreshView();
        $("#say")?.focus();
        return;
      }
      case "goal-add": case "goal-drop": {
        const body = ds.act === "goal-add" ? { text: ($("#goal-in")?.value || "").trim() } : { drop: ds.goal };
        if (ds.act === "goal-add" && !body.text) return $("#goal-in")?.focus();
        t.disabled = true;
        try {
          const r = await api("POST", `/campaigns/${S.route}/me/goals`, body);
          S.view = r.campaign;
        } catch (e) { toast(e.message); t.disabled = false; return; }
        return renderPanel();
      }
      case "auto-roll": {
        const auto = !S.view.you?.auto_roll;
        await api("POST", `/campaigns/${S.route}/me/dice`, { auto_roll: auto });
        toast(auto ? "The game rolls your dice from now on." : "You roll your own dice again.");
        return refreshView();
      }
      case "new-char": S.draft = blankDraft(); S.prev = null; S.step = 0; return renderBuilder();
      case "save-choices": {
        const b = mine()?.build;
        t.disabled = true;
        for (const ch of b?.pending_choices || []) {
          const picks = S._sc?.[ch.key] || [];
          if (picks.length) await api("POST", `/campaigns/${S.route}/me/choose`, { key: ch.key, picks });
        }
        S._sc = {};
        await refreshView(); refreshJournal();
        return toast("Saved.");
      }
      case "prep-edit": S.prepFor = mine()?.id; S._prep = (mine()?.build?.spells || []).map(x => x.key).filter(k => mine().build.prepare.options.includes(k)); return refreshJournal();
      case "prep-cancel": S.prepFor = ""; return refreshJournal();
      case "prep-save":
        t.disabled = true;
        await api("POST", `/campaigns/${S.route}/me/spells`, { spells: S._prep });
        S.prepFor = "";
        await refreshView(); return refreshJournal();
      case "dm-page": S.dmPage = ds.k; S.dmConfirm = null; if (ds.k === "history") S.dmHist = null; return refreshJournal();
      case "dm-ask": S.dmConfirm = +ds.seq; return refreshJournal();
      case "dm-cancel": S.dmConfirm = null; return refreshJournal();
      case "dm-rewind":
        t.disabled = true;
        await api("POST", `/campaigns/${S.route}/dm/rewind`, { seq: +ds.seq });
        S.dmConfirm = null;
        return restartFeed();
      case "dm": case "dm-give": return dmAct(ds, t);
      case "jtab": J.tab = ds.k; J.q = ""; J.editing = ""; return refreshJournal();
      case "note-edit": J.editing = ds.id; refreshJournal(); return $("#n-edit")?.focus();
      case "note-cancel": J.editing = ""; return refreshJournal();
      case "note-del":
        await api("DELETE", `/campaigns/${S.route}/notes/${ds.id}`);
        J.notes = J.notes.filter(n => n.id !== ds.id); J.editing = ""; return refreshJournal();
      case "jot":
        loadNotes();
        await addNote(`“${ds.text.slice(0, 3900)}”`);
        t.classList.add("done"); return toast("Copied to your notes.");
    }
  } catch (er) { toast(er.message); if (S.view?.status === "lobby") renderLobby(); }
});

document.addEventListener("keydown", e => { if (e.key === "Escape" && S.panel) closePanel(); });

// ---- boot

(async () => {
  const g = document.createElement("div"); g.className = "grain"; document.body.appendChild(g);
  try {
    const r = await fetch("/auth/me");
    if (r.ok) S.me = await r.json();
  } catch {}
  route();
})();
