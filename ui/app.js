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
const ICON = {
  back: svg(`<path d="M15 18l-6-6 6-6"/>`),
  close: svg(`<path d="M6 6l12 12M18 6L6 18"/>`, 2.2),
  menu: svg(`<path d="M4 7h16M4 12h16M4 17h10"/>`),
  send: svg(`<path d="M5 12h14M13 6l6 6-6 6"/>`, 2.4),
  plus: svg(`<path d="M12 5v14M5 12h14"/>`),
};
const mark = (cls = "") => `<span class="hex mark ${cls}"></span>`;

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
  stageBoard: false,
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
    talkTo: "", asked: {}, tray: "", hint: {}, composer: "", collapsed: false, stageBoard: false, open: {}, journal: null, map: null, stock: {},
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
  if (!S.meta) api("GET", "/meta").then(m => { S.meta = m; if (!S.route && !S.writing && $("#eras")) $("#eras").outerHTML = erasHTML(); }).catch(() => {});
  app.innerHTML = `<div class="scrollpage" id="home"><div class="home-body top"><section><div class="shelf">${`<div class="book skeleton"><div class="cov"></div></div>`.repeat(3)}</div></section></div></div>`;
  let mine, all;
  try {
    [mine, all] = await Promise.all([api("GET", "/me/campaigns"), api("GET", "/campaigns")]);
  } catch (e) { $("#home").innerHTML = `<div class="center-msg"><div class="error">${esc(e.message)}</div></div>`; return; }
  if (S.route) return;
  const order = { active: 0, lobby: 1, completed: 2, failed: 2 };
  const stories = mine.campaigns.slice().sort((a, b) => (order[a.status] ?? 3) - (order[b.status] ?? 3) || b.updated_at - a.updated_at);
  const lead = stories.find(c => c.status === "active" || c.status === "lobby");
  const rest = stories.filter(c => c !== lead);
  const mineIds = new Set(stories.map(c => c.id));
  const open = all.campaigns.filter(c => c.status === "lobby" && !mineIds.has(c.id) && c.players < c.max_players);
  const top = `<div class="hero-top"><span class="brand">${mark()}aidnd</span><button class="chip-scrim" data-act="signout">${esc(S.me.name)} · Sign out</button></div>`;
  $("#home").innerHTML = `
    ${lead ? `<div class="hero">
      <div class="home-art">${coverArt(lead.id)}</div>${top}
      <div class="home-lead"><div class="col">
        <span class="eyebrow-acc">${lead.status === "lobby" ? "Gathering the party" : "In play"} · ${lead.players}/${lead.max_players} players · ${esc(ago(lead.updated_at))}</span>
        <h1 class="title-xl">${esc(lead.title)}</h1>
        ${lead.pitch ? `<p class="lede">${esc(clip(lead.pitch, 180))}</p>` : ""}
        <div class="hero-cta"><button class="btn primary" data-open="${esc(lead.id)}">${lead.status === "lobby" ? "Back to the table" : "Continue the story"}</button></div>
      </div></div>
    </div>` : `<div class="hero" style="height:64px">${top}</div>`}
    <div class="home-body">
      ${rest.length ? `<section><span class="mono-k">Your other stories</span><div class="shelf">${rest.map(bookHTML).join("")}</div></section>` : ""}
      ${open.length ? `<section><span class="mono-k">Open tables</span><div class="shelf">${open.map(bookHTML).join("")}</div></section>` : ""}
      <section id="new">${newStoryHTML()}</section>
    </div>`;
}

function bookHTML(c) {
  const meta = { lobby: "Gathering the party", active: `In play · ${ago(c.updated_at)}`, completed: "Finished", failed: "Lost" }[c.status] || "";
  return `<button class="book" data-open="${esc(c.id)}"><div class="cov">${coverArt(c.id)}</div><b>${esc(c.title)}</b><span>${esc(meta)}${c.status === "lobby" ? ` · ${c.players}/${c.max_players}` : ""}</span></button>`;
}

function erasHTML() {
  const list = [["", "Any"], ...(S.meta?.settings || []).map(s => [s.key, SETTING_WORD[s.key] || s.name])];
  return `<div class="eras" id="eras">${list.map(([k, l]) => `<button class="${S.era === k ? "on" : ""}" data-era="${esc(k)}">${esc(l)}</button>`).join("")}</div>`;
}

function newStoryHTML() {
  const sug = SUGGESTIONS[hash(S.me?.name || "") % SUGGESTIONS.length];
  return `<span class="mono-k">Begin something new</span>
    <textarea class="under big" id="premise" rows="2" placeholder="What is this story about? A place, a mood, a mystery…"></textarea>
    ${erasHTML()}
    <div class="new-foot"><button class="try" data-premise="${esc(sug)}">Try: <em>${esc(sug)}</em></button><button class="btn" data-act="create">Write this story</button></div>
    <span class="help">The DM writes the places, the people and a main quest. It takes about a minute. To join a friend’s story, open it from Open tables once they have started one.</span>`;
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
    const c = await api("POST", "/campaigns", { premise });
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

function connect(id) {
  if (S.es) return;
  const es = new EventSource(`/api/campaigns/${id}/events`);
  const types = ["campaign_created", "player_joined", "character_created", "player_action", "action_resolved", "narration", "enemy_turns", "quest_updated", "location_changed", "turn_started", "turn_budget", "campaign_ended"];
  types.forEach(t => es.addEventListener(t, ev => onEvent(JSON.parse(ev.data))));
  // Progress events are not saved and have no id: they say the action is
  // being read, then carry its dice while the narration is written.
  es.addEventListener("progress", ev => {
    S.progress = JSON.parse(ev.data).data;
    (S.progress?.rolls || []).forEach(r => queueRoll(r, actorName(S.progress.character_id)));
    renderFeed();
  });
  S.es = es;
}

let refreshTimer, feedTimer;
function onEvent(e) {
  if (e.id <= S.lastId) return;
  S.lastId = e.id;
  S.events.push(e);
  if (e.type !== "character_created" && e.type !== "player_joined") S.progress = null;
  if (fresh(e)) liveEvent(e);
  clearTimeout(feedTimer);
  feedTimer = setTimeout(renderFeed, 30);
  clearTimeout(refreshTimer);
  refreshTimer = setTimeout(refreshView, 250);
}

// liveEvent reacts to something that just happened (not to the replay).
function liveEvent(e) {
  const d = e.data || {};
  if (e.type === "action_resolved") eachRoll(d).forEach(r => queueRoll(r.d20, actorName(d.character_id)));
  if (e.type === "enemy_turns") (d.turns || []).forEach(t => t.attack?.roll && queueRoll({ ...t.attack.roll, kind: "attack", label: t.attack.roll.label || `Attack on ${t.target || "the party"}` }, t.enemy, true));
  if (e.type === "location_changed") { S.collapsed = false; S.talkTo = ""; S.stock = {}; }
  if (e.type === "turn_started" && d.player_id !== S.me?.player_id) S.tray = "";
}

const FRESH_SECS = 30;
const fresh = e => e.created_at && Date.now() / 1000 - e.created_at < FRESH_SECS;

async function refreshView() {
  if (!S.route) return;
  const before = S.view?.status;
  try { S.view = await api("GET", `/campaigns/${S.route}`); } catch { return; }
  S.journal = null; S.map = null;
  if (S.view.encounter && S.talkTo) S.talkTo = "";
  if (before !== S.view.status) return render();
  if (S.view.status === "lobby") return renderLobby(true);
  renderChrome();
}

function render() {
  if (!S.view) return;
  if (S.view.status === "lobby") return renderLobby();
  renderPlay();
}

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
const settingName = () => (SETTING_WORD[S.view?.setting] || S.view?.setting || "").toUpperCase();

function renderLobby(soft) {
  const v = S.view, me = mine();
  if (S.draft && v.you?.joined) {
    if (soft) return;
    return renderBuilder();
  }
  const ready = v.players.length > 0 && v.players.every(p => p.character_id);
  const waiting = v.players.filter(p => !p.character_id).map(p => p.name);
  const status = !v.you?.joined ? "Take a seat to play." : !me ? "Make your character to get ready." : ready ? "Everyone is ready." : `Waiting for ${waiting.join(", ")}…`;
  app.innerHTML = `
  <div class="lobby">
    <div class="lobby-scroll">
      <div class="lobby-art">${coverArt(S.route)}<button class="round" data-act="home" aria-label="Back home">${ICON.back}</button></div>
      <div class="lobby-body">
        <div><span class="mono-k">A new story${settingName() ? " · " + esc(settingName()) : ""}</span><h1 class="title-l">${esc(v.title)}</h1><p class="pitch">${esc(v.pitch || v.premise)}</p></div>
        <section><span class="mono-k">Around the table</span><div class="seats" id="seats">${seatsHTML()}</div></section>
        ${v.you?.joined ? `<section><span class="mono-k">Your character</span>${me ? meCardHTML(me) : conceptHTML()}</section>` : ""}
      </div>
    </div>
    <div class="foot-bar"><div><span class="status">${esc(status)}</span>
      ${v.you?.joined ? `<button class="btn primary ${ready ? "" : "off"}" data-act="start">Begin the story</button>` : `<button class="btn primary" data-act="join">Take a seat as ${esc(S.me.name)}</button>`}
    </div></div>
  </div>`;
}

function seatsHTML() {
  const v = S.view;
  return Array.from({ length: v.max_players }, (_, i) => v.players[i]).map(p => {
    if (!p) return `<div class="seat open"><span class="face">+</span><b>Open seat</b><span>share the play code</span></div>`;
    const c = v.party.find(x => x.id === p.character_id), you = p.id === S.me?.player_id;
    return `<div class="seat ${c ? "ready" : ""} ${you ? "you" : ""}">${c ? face(c) : `<span class="face">${initial(p.name)}</span>`}
      <b>${esc(p.name)}</b><span>${c ? `${esc(firstName(c.name))}, ${esc(kitName(c.kit).toLowerCase())}` : "making a character…"}</span></div>`;
  }).join("");
}

function meCardHTML(c) {
  return `<div class="me-card">${face(c)}<div>
    <span class="name-l">${esc(c.name)}</span>
    <span class="sub-line">${esc(c.class)} · ${esc(kitName(c.kit))} · ${esc(c.pronouns || "they/them")}</span>
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

const STEPS = ["Who", "Kit", "Abilities", "Story"];
const STEP_TITLE = ["Who are they?", "What do they carry?", "What are they good at?", "Where do they come from?"];

function blankDraft() {
  const arr = S.meta?.standard_array || [15, 14, 13, 12, 10, 8];
  return { name: "", class: "", kit: kitList()[0]?.key || "", pronouns: "they/them", concept: $("#concept")?.value.trim() || "", backstory: "",
    scores: Object.fromEntries(ABIL.map((a, i) => [a, arr[i]])), proficient: [] };
}

function renderBuilder() {
  const d = S.draft, i = S.step, v = S.view;
  const body = [
    () => `<div class="grid-who">
        <label class="field"><span>Name</span><input class="under" id="d-name" value="${esc(d.name)}" maxlength="40"></label>
        <label class="field"><span>Pronouns</span><input class="under" id="d-pron" value="${esc(d.pronouns || "they/them")}" maxlength="20"></label>
      </div>
      <label class="field"><span>Calling</span><input class="under" id="d-class" value="${esc(d.class)}" placeholder="Private eye, occultist, boxer…"><span class="help">What the story calls them. Your kit decides the gear.</span></label>`,
    () => `<div class="kits">${kitList().map(k => `<button class="kit ${k.key === d.kit ? "on" : ""}" data-kit="${esc(k.key)}"><span class="dot"></span><span class="txt">
        <span class="nm"><b>${esc(k.name)}</b><span>${esc(k.desc)}</span></span>
        <span class="items">${esc((k.items || []).map(it => it.name + (it.damage ? ` · ${it.damage}` : "")).join(" · "))}</span></span></button>`).join("")}</div>`,
    () => `<p class="intro">Tap two numbers to swap them. Star two strengths: those rolls add your proficiency.</p>
      <div class="abil-grid">${ABIL.map(a => {
        const val = d.scores?.[a] ?? 10, prof = (d.proficient || []).includes(a);
        return `<div class="abil ${prof ? "prof" : ""} ${S.swap === a ? "sel" : ""}">
          <button class="swap" data-swap="${a}" aria-label="${ABIL_NAME[a]} ${val}"><span class="k">${a}</span><span class="v">${val}</span><span class="m">${signed(mod(val))}${prof ? " +2" : ""}</span></button>
          <button class="star" data-prof="${a}" aria-label="Proficient in ${ABIL_NAME[a]}">${prof ? "★" : "☆"}</button></div>`;
      }).join("")}</div>`,
    () => `<label class="field"><span>In a sentence</span><textarea class="under text" id="d-concept" rows="2">${esc(d.concept)}</textarea></label>
      <label class="field"><span>Backstory · optional</span><textarea class="under text" id="d-back" rows="3" placeholder="Where they come from, what they want, who they owe.">${esc(d.backstory)}</textarea></label>
      <div class="summary"><span class="face">${initial(d.name)}</span><div>
        <span class="nm">${esc(d.name || "Unnamed")}</span>
        <span class="sub-line">${esc(d.class || "No calling yet")} · ${esc(kitName(d.kit))} · ${esc(d.pronouns || "they/them")}</span>
        <span class="stats">${ABIL.map(a => `${a} ${d.scores[a]}${(d.proficient || []).includes(a) ? "★" : ""}`).join("  ")}</span>
        <span class="stats">${esc((kit(d.kit)?.items || []).map(it => it.name).join(" · "))}</span></div></div>`,
  ][i]();
  app.innerHTML = `
  <div class="builder">
    <div class="builder-head"><div>
      <div class="top"><button class="x" data-act="cancel-draft" aria-label="Stop making a character">${ICON.close}</button><span class="mono-k">Your character · ${esc(v.title)}</span></div>
      <div class="tabs">${STEPS.map((s, n) => `<button class="${n === i ? "on" : ""}" data-step="${n}">${s}</button>`).join("")}</div>
    </div></div>
    <div class="builder-body"><div><h2>${STEP_TITLE[i]}</h2>${body}<div class="error" id="d-err"></div></div></div>
    <div class="builder-foot"><div>
      <button class="back" data-act="${i === 0 ? "cancel-draft" : "step-back"}">${i === 0 ? "Cancel" : "Back"}</button>
      <button class="btn primary" data-act="${i < STEPS.length - 1 ? "step-next" : "save-char"}">${i < STEPS.length - 1 ? "Next" : "Save character"}</button>
    </div></div>
  </div>`;
}

function readEditor() {
  const d = S.draft;
  if (!d) return d;
  const val = id => $(id)?.value;
  if ($("#d-name")) { d.name = val("#d-name"); d.pronouns = val("#d-pron"); d.class = val("#d-class"); }
  if ($("#d-back")) { d.backstory = val("#d-back"); d.concept = val("#d-concept"); }
  return d;
}

// ---- play

function renderPlay() {
  const stage = stageMode();
  app.innerHTML = `
  <div class="play ${stage ? "stage" : ""} ${sideMode() ? "side" : ""}" id="play">
    ${stage ? `<div class="stage-art" id="top"></div>` : ""}
    <div class="storycol">
      ${stage ? "" : `<div class="banner ${S.collapsed ? "collapsed" : ""}" id="top"></div>`}
      <div class="feedwrap"><div class="feed-scroll" id="scroll"><div class="feed" id="feed"></div></div><div class="die-layer" id="die"></div></div>
      <footer class="dock" id="dock"></footer>
    </div>
    <div id="dimmer"></div>
    <aside class="panel" id="panel" aria-label="Panel"></aside>
  </div>`;
  $("#scroll").addEventListener("scroll", onFeedScroll, { passive: true });
  renderChrome();
  renderFeed(true);
  renderDie();
}

function renderChrome() {
  if (!$("#play")) return;
  renderTop(); renderDock(); renderPanel();
}

function sceneLine(upper) {
  const v = S.view, me = mine(), loc = v.location || {};
  const zone = loc.zones?.find(z => z.id === (me || v.party[0])?.zone_id)?.name;
  const when = v.scene ? `Day ${v.scene.day}, ${v.scene.time}` : "";
  const s = [zone || loc.kind, when].filter(Boolean).join(" · ");
  return upper ? s.toUpperCase() : s;
}

function renderTop() {
  const el = $("#top");
  if (!el) return;
  const v = S.view, loc = v.location || {}, enc = v.encounter;
  const when = v.scene ? `Day ${v.scene.day}, ${v.scene.time}` : "";
  if (!stageMode()) {
    el.innerHTML = `${placeArt(loc.id)}<div class="shade"></div>
      <div class="banner-in">
        <div class="bar">
          <button class="round" data-act="home" aria-label="Back home">${ICON.back}</button>
          <span class="t"><b class="when-o">${esc(v.title)}</b><b class="when-c">${esc(loc.name)}</b><span class="when-o">${esc(when)}</span><span class="when-c">${esc(sceneLine())}</span></span>
          ${enc ? `<span class="fight">R${enc.round}</span>` : ""}
          <button class="round" data-act="menu" aria-label="Open the scene, journal and map">${ICON.menu}</button>
        </div>
        <div class="place-t"><span class="mono-k">${esc(sceneLine(true))}</span><b>${esc(loc.name)}</b></div>
      </div>`;
    return;
  }
  const btn = (k, label, on) => `<button class="pill-btn ${on ? "on" : ""}" data-act="${k === "board" ? "board" : "panel"}" data-k="${k}">${label}</button>`;
  el.innerHTML = `${placeArt(loc.id)}<div class="shade"></div><div class="fade-r"></div>
    <div class="stage-top">
      <button class="round" data-act="home" aria-label="Back home">${ICON.back}</button>
      <span class="tchip"><b>${esc(v.title)}</b><span>${esc(when)}</span></span>
      <span style="flex:1"></span>
      ${enc ? `<span class="fight">FIGHT · ROUND ${enc.round}</span>` : ""}
      ${btn("board", S.stageBoard && !enc ? "Hide board" : "Board", S.stageBoard || enc)}${btn("journal", "Journal", S.panel === "journal")}${btn("map", "Map", S.panel === "map")}
    </div>
    ${S.stageBoard || enc ? `<div class="stage-board">${boardHTML()}</div>` : ""}
    <div class="stage-foot">
      <div class="where"><span class="mono-k">${esc(sceneLine(true))}</span><b>${esc(loc.name)}</b></div>
      <div class="portraits">${v.party.map(c => {
        const acting = v.active?.character_id === c.id;
        return `<button class="ptile ${acting ? "acting" : ""} ${isMe(c.id) ? "you" : ""} ${c.down ? "down" : ""}" data-sheet="${esc(c.id)}" title="${esc(c.name)} · ${c.hp}/${c.max_hp} HP">
          ${face(c)}<span>${esc(firstName(c.name))}</span>${hpline(c)}</button>`;
      }).join("")}</div>
    </div>`;
}

const hpPct = c => Math.max(0, Math.min(100, Math.round(100 * c.hp / (c.max_hp || 1))));
const hpline = c => `<span class="hpline"><i class="${c.hp / c.max_hp <= .5 ? "low" : ""}" style="width:${hpPct(c)}%"></i></span>`;

// The banner folds away once the story is scrolled, and opens again only at
// the top of a feed long enough to scroll (so it never flickers).
let stick = true;
function onFeedScroll(e) {
  const el = e.target, top = el.scrollTop;
  stick = el.scrollHeight - top - el.clientHeight < 80;
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
  const lastNarr = items.map(it => it.k === "narr").lastIndexOf(true);
  feed.innerHTML = items.map((it, i) => itemHTML(it, i === lastNarr)).join("") + pendingHTML() + suggestHTML();
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
    out.push(r.kind === "damage" || r.kind === "heal" ? { d20: null, dmg: r } : { d20: r });
  }
  return out;
}

// feedItems turns events into what the story shows: places, whose turn it
// is, what players did, the dice, the narration and quest news.
function feedItems(evs) {
  const out = [], party = S.view?.party || [];
  let lastTurn = "", joins = [];
  const flush = () => { if (joins.length) out.push({ k: "sys", text: `${list(joins)} ${joins.length > 1 ? "join" : "joins"} the party` }); joins = []; };
  for (const e of evs) {
    const d = e.data || {};
    if (e.type === "character_created") { joins.push(`${d.name} the ${d.class}`); continue; }
    flush();
    switch (e.type) {
      case "location_changed": out.push({ k: "place", name: d.name }); break;
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
        rolls.forEach((g, i) => out.push(rollItem(`${e.id}.${i}`, who, g, false, i === rolls.length - 1 ? changes : [], actorName(d.character_id))));
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
          if (!t.attack?.roll) return;
          const g = { d20: { ...t.attack.roll, kind: "attack", label: t.attack.roll.label || `vs ${t.target || "the party"}` }, dmg: t.attack.damage ? { ...t.attack.damage, kind: "damage" } : null };
          out.push(rollItem(`${e.id}.e${i}`, t.enemy, g, true, t.text ? [t.text] : []));
        });
        if (d.narration) out.push({ k: "narr", text: d.narration });
        break;
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
// the("The Shade") is "The Shade", the("Goblin") is "the Goblin".
const the = n => /^(the|a|an)\s/i.test(n) ? n : `the ${n}`;
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
    case "sys": return `<div class="ev-sys ${it.bad ? "bad" : ""}">${esc(it.text)}</div>`;
    case "place": return `<div class="ev-place"><span>${esc(it.name)}</span></div>`;
    case "turn": return `<div class="ev-turn ${it.you ? "you" : ""}">· ${it.you ? "Your turn" : `${esc(it.name)}’s turn`} ·</div>`;
    case "act": return actHTML(it.c, it.you, it.text);
    case "narr": return narrHTML(it, latest);
    case "roll": return rollHTML(it);
    case "quest": return `<div class="ev-quest"><b>${esc(it.label)}</b><span>${esc(it.text)}</span></div>`;
  }
  return "";
}

function actHTML(c, you, text) {
  return `<div class="ev-act ${you ? "you" : ""}">${face(c, "round-f")}<p><b>${you ? "You" : esc(c?.name || "")}</b> “${esc(String(text || "").replace(/^["“]|["”]$/g, ""))}”</p></div>`;
}

// Narration comes as plain text. Paragraphs split on blank lines; a
// paragraph that is only a quoted line reads as speech; the opening's first
// paragraph is the lead.
function narrHTML(it, latest) {
  const paras = String(it.text).split(/\n\s*\n|\n/).map(p => p.trim()).filter(Boolean);
  return `<div class="ev-narr ${latest ? "latest" : ""} ${it.quick ? "quick" : ""}">${paras.map((p, i) => {
    if (it.opening && i === 0) return `<p class="lead">${esc(p)}</p>`;
    const q = p.match(/^[“"](.+)[”"]$/s);
    return q ? `<div class="say"><p>“${esc(q[1])}”</p></div>` : `<p>${esc(p)}</p>`;
  }).join("")}</div>`;
}

const TIER = {
  attack: { critical_success: "CRIT", success: "HIT", failure: "MISS", critical_failure: "FUMBLE" },
  check: { critical_success: "CRITICAL", success: "SUCCESS", failure: "FAIL", critical_failure: "FUMBLE" },
};
const okTier = t => t === "success" || t === "critical_success";

function rollHTML(it) {
  const r = it.d20, dm = it.dmg;
  if (!r) {
    return `<div class="roll dmg"><div class="roll-line"><span class="hex">${dm.total}</span><span class="what">${esc(it.who)} · ${esc(dm.label || dm.kind)} · ${esc(dm.expr || "")}</span><span class="tier">${dm.kind === "heal" ? "HEALED" : "DAMAGE"}</span></div></div>`;
  }
  if (DIE.isPending(it.sig)) return `<div class="ev-rolling">rolling…</div>`;
  const ok = okTier(r.tier), open = !!S.open[it.id];
  const words = TIER[r.kind === "attack" ? "attack" : "check"];
  const parts = r.parts?.length ? r.parts : r.mod ? [{ label: "Modifier", value: r.mod }] : [];
  const against = r.against === "AC" || r.kind === "attack" ? "Armor" : "Difficulty";
  return `<div class="roll ${ok ? "ok" : "no"} ${it.foe ? "foe" : ""}">
    <button class="roll-line" data-roll="${esc(it.id)}" aria-expanded="${open}">
      <span class="hex">${r.natural}</span>
      <span class="what">${it.who ? esc(it.who) + " · " : ""}${esc(r.label)} · ${r.total} vs ${r.target}${dm ? ` · ${dm.total} dmg` : ""}</span>
      <span class="tier">${words[r.tier] || ""}</span><span class="chev">${open ? "–" : "+"}</span>
    </button>
    ${open ? `<div class="breakdown">
      <div><span>d20</span><span>${r.natural}</span></div>
      ${parts.map(p => `<div><span>${esc(p.label)}</span><span>${signed(p.value)}</span></div>`).join("")}
      <div class="total"><span>Total</span><span>${r.total}</span></div>
      <div><span>${against}${r.cover ? ` (+${r.cover} cover)` : ""}</span><span>${r.target}</span></div>
      ${dm ? `<div><span>Damage ${esc(dm.expr || "")}${dm.dice?.length ? ` [${dm.dice.join(", ")}]` : ""}${r.crit || r.tier === "critical_success" ? " · doubled" : ""}</span><span>${dm.total}</span></div>` : ""}
      ${(it.changes || []).map(c => `<span class="chg">${esc(c)}</span>`).join("")}
    </div>` : ""}
  </div>`;
}

// pendingHTML shows the action in flight: what was said, the dice once
// they have landed, and which step the game master is on.
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
    if (t === "narration" || t === "enemy_turns" || t === "turn_started" || t === "turn_budget" || t === "campaign_ended") return false;
    if (t === "player_action") return true;
  }
  return false;
}

// ---- suggested next steps: two lines under the story on your turn while
// the input is empty. They are guesses from the view; the engine decides.

function suggestions() {
  const v = S.view, me = mine(), loc = v.location || {}, b = v.budget || {};
  if (!v.you?.your_turn || isThinking() || (S.composer || "").trim() || !me) return [];
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
  return sg.length ? `<div class="suggest">${sg.map((s, i) => `<button data-sg="${i}"><i>→</i><b>${esc(s.text)}</b></button>`).join("")}</div>` : "";
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
    <div class="spinner-die"><span class="hex"><span class="face-n">${land ? r.natural : 1 + Math.floor(Math.random() * 20)}</span></span></div>
    <div class="die-cap"><span class="mono-k">${esc(d.who)} · ${esc(shortLabel(r.label, d.name))}</span>
      ${land ? `<span class="res">${words}  ${r.total} vs ${r.target}</span>` : `<span class="need">d20 ${esc(parts)} · needs ${r.target}</span>`}</div>
  </div></div>`;
}

// ---- dock

function renderDock() {
  const v = S.view, el = $("#dock");
  if (!el) return;
  const me = mine();
  if (v.status === "completed" || v.status === "failed") {
    el.innerHTML = `<div class="dock-in"><div class="ended"><span>${v.status === "completed" ? "The story is complete." : "The story has ended."}</span><button class="btn" data-act="home">Back home</button></div></div>`;
    return;
  }
  const yours = !!v.you?.your_turn, thinking = isThinking(), enc = v.encounter;
  const groups = trays();
  if (S.tray && !groups[S.tray]?.items.length) S.tray = "";
  if (!yours) S.tray = "";
  const defs = enc ? [["attack", "Attack"], ["move", "Move"], ["roll", "Roll"], ["use", "Use"], ["end", "End turn"]]
    : [["look", "Look"], ["talk", "Talk"], ["go", "Go"], ["roll", "Roll"], ["use", "Use"], ["end", "End turn"]];
  const acts = defs.filter(([k]) => k === "look" || k === "end" || groups[k]?.items.length)
    .map(([k, l]) => `<button class="${S.tray === k ? "on" : ""}" data-dock="${k}" ${thinking ? "disabled" : ""}>${l}</button>`).join("");
  const active = v.active, ap = active && v.players.find(p => p.id === active.player_id);
  const idle = v.turn_started ? Date.now() / 1000 - v.turn_started : 0;
  const talkN = S.talkTo && (v.location?.npcs || []).find(n => n.id === S.talkTo);
  const kept = $("#say")?.value ?? S.composer;
  el.innerHTML = `<div class="dock-in">
    ${enc ? `<div class="foes">${enc.enemies.map(e => `<button class="foe-card ${e.hp > 0 ? "" : "dead"}" ${e.hp > 0 ? `data-attack="${esc(e.id)}"` : "disabled"}>
      <span class="top"><b>${esc(e.display)}</b><span class="hp">${Math.max(0, e.hp)}/${e.max_hp}</span></span>
      <span class="bar3"><i style="width:${Math.max(0, Math.round(100 * e.hp / e.max_hp))}%"></i></span>
      <span class="cta">${e.hp <= 0 ? "down" : yours && v.budget?.action !== 0 ? "Tap to attack" : e.ranged ? "ranged" : zoneName(e.zone_id)}</span></button>`).join("")}</div>` : ""}
    ${yours && S.tray ? trayHTML(groups[S.tray]) : ""}
    ${yours ? `<div class="turnrow"><span class="who">Your turn</span><span class="acts">${acts}</span></div>` : active ? `<div class="turnrow"><span class="who other">${esc(active.name)}’s turn <em>· ${esc(ap?.name || "")} is thinking</em></span>
      ${v.you?.joined && idle > 120 ? `<span class="acts"><button data-act="skip">Skip their turn</button></span>` : ""}</div>` : ""}
    ${yours ? checkChipHTML() : ""}
    <form class="inputbar ${yours && !thinking ? "you" : ""}" id="say-form">
      <div class="party">${v.party.map(c => `<button type="button" class="pa ${v.active?.character_id === c.id ? "acting" : ""} ${isMe(c.id) ? "you" : ""}" data-sheet="${esc(c.id)}" title="${esc(c.name)} · ${c.hp}/${c.max_hp} HP">${initial(c.name)}${face(c, "round-f")}<span class="hpl"><i class="${c.hp / c.max_hp <= .5 ? "low" : ""}" style="width:${hpPct(c)}%"></i></span></button>`).join("")}</div>
      <span class="sep"></span>
      ${thinking ? `<span class="status">The dice are rolling…</span>`
        : yours ? `<textarea id="say" rows="1" enterkeyhint="send" placeholder="${esc(enc ? "Say exactly what you do…" : talkN ? `Say something to ${talkN.name}…` : "What do you do?")}">${esc(kept)}</textarea>
          <button class="send" aria-label="Send">${ICON.send}</button>`
        : `<span class="status">${active ? `Waiting for ${esc(active.name)}…` : "Waiting…"}</span>`}
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

function trays() {
  const v = S.view, loc = v.location || {}, me = mine(), enc = !!v.encounter, b = v.budget || {};
  const g = { talk: { title: "Talk to", items: [] }, go: { title: "Go to", items: [] }, move: { title: "Move to · uses your move", items: [] }, attack: { title: "Attack", items: [] }, roll: { title: "Roll a check", items: [] }, use: { title: "Use", items: [] } };
  if (!me) return g;
  for (const n of loc.npcs || []) if (n.alive !== false) g.talk.items.push({ name: n.name, sub: [n.role, zoneName(n.zone_id)].filter(Boolean).join(" · "), talk: n.id });
  for (const z of loc.zones || []) if (z.id !== me.zone_id) {
    const it = { name: z.name, sub: enc ? (b.move === 0 ? "no move left" : "uses your move") : "nearby", send: `I move to ${z.name}`, hint: { zone_id: z.id } };
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
  }
  // Checks a player can call for. Search and locks are rolled exactly as
  // picked; an ability check keeps the ability and lets the game master set
  // the DC from what the player writes.
  const tools = (me.inventory || []).some(i => i.key === "thieves_tools");
  g.roll.items.push({ name: "Search for hidden ways", bonus: `WIS ${signed(checkMod(me, "WIS"))}`, send: "I search the area for hidden ways", hint: { check: { ability: "WIS", purpose: "search" } } });
  for (const x of loc.exits || []) {
    if (x.state !== "locked" || x.sealed) continue;
    g.roll.items.push({ name: `Pick the lock to ${x.to_name}`, bonus: `DEX ${signed(checkMod(me, "DEX") + (tools ? 2 : 0))}`, send: `I pick the lock on the way to ${x.to_name}`, hint: { check: { ability: "DEX", purpose: "unlock", exit: x.id } } });
    g.roll.items.push({ name: `Force the way to ${x.to_name}`, bonus: `STR ${signed(checkMod(me, "STR"))}`, send: `I force the way to ${x.to_name}`, hint: { check: { ability: "STR", purpose: "force", exit: x.id } } });
  }
  g.roll.abil = true;
  for (const i of loc.items || []) if (!enc || b.free !== 0) g.use.items.push({ name: `Take the ${i.name}`, sub: "on the ground", send: `I pick up ${the(i.name)}` });
  for (const i of me.inventory || []) if (i.kind === "consumable") g.use.items.push({ name: i.name, sub: effectText(i.effect), send: `I use the ${i.name}` });
  if (me.hp < me.max_hp && !enc) g.use.items.push({ name: "Rest a while", sub: "recover", send: "I rest for a while" });
  return g;
}

function trayHTML(t) {
  const me = mine();
  S._tray = t.items;
  return `<div class="tray"><span class="mono-k">${esc(t.title)}</span>
    ${t.items.map((o, i) => `<button class="tray-item" data-pick="${i}"><b>${esc(o.name)}</b>${o.bonus ? `<span class="bonus">${esc(o.bonus)}</span>` : o.sub ? `<span>${esc(o.sub)}</span>` : ""}</button>`).join("")}
    ${t.abil && me ? `<span class="mono-k">Any other check · say what you try, the DM sets the difficulty</span>
      <div class="abil-strip">${ABIL.map(a => `<button data-check="${a}"><small>${a}</small><b>${signed(checkMod(me, a))}</b></button>`).join("")}</div>` : ""}
  </div>`;
}

function checkChipHTML() {
  const c = S.hint?.check, me = mine();
  if (!c || !me) return "";
  const what = { search: "Search", unlock: "Pick the lock", force: "Force it", other: `${ABIL_NAME[c.ability] || c.ability} check` }[c.purpose] || "Check";
  return `<div class="check-chip"><span class="hex">d20</span><b>${esc(what)}</b><span>${esc(c.ability)} ${signed(checkMod(me, c.ability))}</span>
    <span class="grow">${c.purpose === "other" ? "say what you try" : "rolled when you send"}</span><button type="button" data-act="clear-check" aria-label="Drop the check">${ICON.close}</button></div>`;
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

const PANEL_TABS = [["scene", "Scene"], ["journal", "Journal"], ["map", "Map"], ["sheet", "Character"]];

function openPanel(k) {
  S.panel = k;
  if (k !== "talk") S.lastPanel = k;
  if (k === "map" && !S.map) loadMap();
  if (k === "journal" && !S.journal) loadJournal();
  renderPanel(); renderTop();
}
function closePanel(quiet) {
  if (!S.panel) return;
  S.panel = null;
  if (!quiet) { renderPanel(); renderTop(); }
  else { const p = $("#panel"); p?.classList.remove("open"); const d = $("#dimmer"); if (d) d.innerHTML = ""; }
}

function renderPanel() {
  const el = $("#panel");
  if (!el) return;
  if (!S.panel) { el.classList.remove("open"); $("#dimmer").innerHTML = ""; return; }
  if (panelBusy()) { J.stale = true; return; }
  const k = S.panel;
  const n = k === "talk" && (S.view.location?.npcs || []).find(x => x.id === S.talkTo);
  if (k === "talk" && !n) { S.panel = null; return renderPanel(); }
  const scroll = el.classList.contains("open") && el.dataset.k === k ? el.scrollTop : 0;
  el.dataset.k = k;
  el.innerHTML = `<div class="panel-head">
      ${k === "talk" ? `<h3>Talking</h3>` : `<div class="ptabs" role="tablist">${PANEL_TABS.map(([t, l]) => `<button role="tab" class="${t === k ? "on" : ""}" data-act="panel" data-k="${t}">${l}</button>`).join("")}</div>`}
      <button class="close" data-act="close-panel" aria-label="Close">${ICON.close}</button></div>
    <div class="panel-body">${({ scene: sceneHTML, talk: () => talkHTML(n), sheet: sheetHTML, journal: journalHTML, map: mapHTML })[k]()}</div>`;
  el.classList.add("open");
  el.scrollTop = scroll;
  $("#dimmer").innerHTML = `<div class="scrim-dim" data-act="close-panel"></div>`;
}

// The panel re-renders on every game event. Hold off while the player is
// typing in it and catch up when they leave the field.
function panelBusy() {
  const a = document.activeElement;
  return !!a && !!a.closest?.("#panel") && a.matches("input, textarea");
}
document.addEventListener("focusout", () => setTimeout(() => { if (J.stale && !panelBusy()) { J.stale = false; renderPanel(); } }, 0));

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
  relax([...Object.values(pos), ...ex.map(e => e.p)], 30, 22, 16, 84, 12, 90);
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
  return `<div class="board">
    <svg class="links" viewBox="0 0 100 75" preserveAspectRatio="none">${L.links.map(l => `<line x1="${l.a.x}" y1="${yy(l.a.y)}" x2="${l.b.x}" y2="${yy(l.b.y)}" ${l.dash ? `stroke-dasharray="1.4 1.2"` : ""}/>`).join("")}</svg>
    ${(loc.zones || []).map(z => `<button class="node ${z.id === me?.zone_id ? "mine" : ""}" style="left:${L.pos[z.id].x}%;top:${L.pos[z.id].y}%" data-zone="${esc(z.id)}" title="${esc(z.name)}${z.desc ? ": " + esc(z.desc) : ""}${z.cover ? " · cover" : ""}">
      <b>${esc(z.name)}${z.cover ? " ◆" : ""}</b><span class="toks">${(toks[z.id] || []).join("")}</span></button>`).join("")}
    ${L.exits.map(({ x, p }) => `<button class="node exit ${x.state === "locked" ? "locked" : ""}" style="left:${p.x}%;top:${p.y}%" data-exit="${esc(x.id)}" title="${esc(x.label)}${x.state === "locked" ? " · locked" : ""}"><b>↗ ${esc(x.to_name)}</b></button>`).join("")}
  </div>`;
}

function sceneHTML() {
  const v = S.view, loc = v.location || {};
  const npcs = (loc.npcs || []).filter(n => n.alive !== false);
  return `<p class="desc">${esc(loc.description)}</p>
    ${loc.zones?.length ? `${boardHTML()}<span class="hint-line">Tap a spot to move${v.encounter ? " (uses your move)" : ""}. Paths at the edge lead elsewhere. ◆ means cover.</span>` : ""}
    <section><span class="mono-k">People here</span>
      ${npcs.map(n => `<button class="person" data-talk="${esc(n.id)}"><span class="face round-f">${initial(n.name)}</span>
        <span class="t"><b>${esc(n.name)}</b><span>${esc([n.role, n.merchant ? "sells goods" : ""].filter(Boolean).join(", "))}${zoneName(n.zone_id) ? " · " + esc(zoneName(n.zone_id)) : ""}</span></span><span class="go">Talk →</span></button>`).join("")
        || `<span class="empty-line" style="padding:8px 0">No one else is here.</span>`}
    </section>
    ${loc.items?.length ? `<section><span class="mono-k">On the ground</span>${loc.items.map(i => `<button class="row-line" data-send="I pick up ${esc(the(i.name))}"><span class="nm">${esc(i.name)}${i.effect ? `<small>${esc(effectText(i.effect))}</small>` : ""}</span><span class="verb">Take</span></button>`).join("")}</section>` : ""}`;
}

// talk. The game has no list of topics, so these are prompts made from what
// the party knows; whatever is asked goes to the game master as words.
function topics(n) {
  const v = S.view, out = [];
  const given = (v.quests || []).filter(q => q.giver_name === n.name && q.status !== "completed");
  given.forEach(q => out.push({ key: "q" + q.id, label: q.title, ask: `about ${q.title.toLowerCase().replace(/^the /, "the ")}` }));
  const main = (v.quests || []).find(q => q.is_main && q.status !== "completed");
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

function sheetHTML() {
  const v = S.view, me = mine();
  const c = v.party.find(p => p.id === S.sheetFor) || me || v.party[0];
  if (!c) return `<p class="empty-line">No one has made a character yet.</p>`;
  const own = isMe(c.id), pl = playerOf(c);
  const xpFloor = c.level_xp || 0, xpP = c.next_level_xp ? Math.max(0, Math.min(1, (c.xp - xpFloor) / (c.next_level_xp - xpFloor))) : 1;
  const worn = new Set([c.weapon?.id, c.armor?.id].filter(Boolean));
  const items = (c.inventory || []).slice().sort((a, b) => (b.equipped || worn.has(b.id)) - (a.equipped || worn.has(a.id)));
  const atk = c.attack || (c.weapon ? { weapon: c.weapon.name, bonus: c.weapon.to_hit, damage: c.weapon.damage, reach: c.weapon.reach } : null);
  return `${v.party.length > 1 ? `<div class="tabs">${v.party.map(p => `<button class="${p.id === c.id ? "on" : ""}" data-sheet="${esc(p.id)}">${esc(firstName(p.name))}${isMe(p.id) ? " (you)" : ""}</button>`).join("")}</div>` : ""}
    <div class="cs-id">${face(c)}<div>
      <span class="nm">${esc(c.name)}</span>
      <span class="sub-line">Level ${c.level} ${esc(c.class)} · ${esc(kitName(c.kit))} · ${esc(c.pronouns || "they/them")}</span>
      <span class="pl">${own ? "You" : pl ? `Played by ${esc(pl.name)}` : ""}${c.zone_name ? ` · at ${esc(c.zone_name)}` : ""}</span>
      ${c.conditions?.length ? `<span class="conds">${c.conditions.map(k => `<span class="cond ${esc(k.key)}" title="${esc(k.note || "")}">${esc(k.label)}</span>`).join("")}</span>` : ""}
    </div></div>
    <div><div class="hp-k"><span class="mono-k">Hit points</span><b>${c.hp} / ${c.max_hp}</b></div><div class="bar5"><i class="${c.hp / c.max_hp <= .34 ? "low" : ""}" style="width:${hpPct(c)}%"></i></div></div>
    <div><div class="vit4">
      <div><span>ARMOR</span><b>${c.ac}</b></div><div><span>GOLD</span><b>${c.gold}</b></div>
      <div><span>LEVEL</span><b>${c.level}</b></div><div><span>XP</span><b>${c.xp}${c.next_level_xp ? `<small>/${c.next_level_xp}</small>` : ""}</b></div></div>
      <div class="bar5 xp"><i style="width:${xpP * 100}%"></i></div>
      ${c.ac_parts?.length ? `<div class="eq">AC ${c.ac_parts.map((p, n) => `${n ? (p.value < 0 ? " − " : " + ") : ""}${esc(p.label)} ${n ? Math.abs(p.value) : p.value}`).join("")} = <b>${c.ac}</b></div>` : ""}
      ${atk ? `<div class="eq">Attack <b>${esc(atk.weapon || "")}</b> ${atk.bonus != null ? signed(atk.bonus) + " to hit" : ""} · ${esc(atk.damage || "")} ${esc(atk.reach || "")}${atk.ability ? ` · ${esc(atk.ability)}` : ""}</div>` : ""}
      ${c.level_up ? `<p class="small-note">Level ${c.level_up.level} in ${Math.max(0, c.next_level_xp - c.xp)} XP: +${c.level_up.hp} max HP${c.level_up.proficiency_bonus > c.proficiency_bonus ? `, proficiency ${signed(c.level_up.proficiency_bonus)}` : ""}.</p>` : ""}
    </div>
    <div class="stat6">${ABIL.map(a => {
      const p = (c.proficient || []).includes(a);
      return `<div class="${p ? "prof" : ""}" title="${ABIL_NAME[a]}${p ? `, proficient (+${c.proficiency_bonus})` : ""}"><span>${a}${p ? " ★" : ""}</span><b>${signed(c.modifiers?.[a] ?? mod(c.scores[a]))}</b><small>${c.scores[a]}</small></div>`;
    }).join("")}</div>
    <section><span class="mono-k">Carrying</span>
      ${items.map(i => {
        const on = i.equipped || worn.has(i.id);
        const verb = !own ? "" : i.kind === "consumable" ? "Use" : !on && ["weapon", "armor", "shield"].includes(i.kind) ? "Equip" : "";
        const note = [on ? (i.kind === "weapon" ? "in hand" : "worn") : "", itemFacts(i)].filter(Boolean).join(" · ");
        const body = `<span class="nm">${esc(i.name)}${note ? `<small>${esc(note)}</small>` : ""}</span>`;
        return verb ? `<button class="row-line" data-send="I ${verb.toLowerCase()} the ${esc(i.name)}">${body}<span class="verb">${verb}</span></button>` : `<div class="row-line">${body}</div>`;
      }).join("") || `<span class="empty-line">Nothing.</span>`}
    </section>
    ${c.backstory || c.concept ? `<section><span class="mono-k">Story</span><p class="story-p">${esc(c.backstory || c.concept)}</p></section>` : ""}`;
}

// journal. Quests, people, places and clues come from the world record on
// the server; notes are the player's own and private.

const J = { tab: "quests", q: "", notes: null, notesFor: "", draft: "", editing: "", stale: false };
const JTABS = [["quests", "Quests"], ["people", "People"], ["places", "Places"], ["clues", "Clues"], ["notes", "Notes"]];

function loadJournal() {
  api("GET", `/campaigns/${S.route}/journal`).then(j => { S.journal = j; if (S.panel === "journal") renderPanel(); }).catch(() => {});
}
function loadNotes() {
  if (J.notesFor === S.route) return;
  J.notesFor = S.route; J.notes = null;
  if (!S.view?.you?.joined) { J.notes = []; return; }
  api("GET", `/campaigns/${S.route}/notes`).then(r => { J.notes = r.notes; if (S.panel === "journal") renderPanel(); }).catch(() => { J.notes = []; });
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
  const find = J.tab === "quests" || J.tab === "notes" ? "" : `<input class="find" id="j-find" type="search" placeholder="Search ${J.tab}" value="${esc(J.q)}" autocomplete="off">`;
  let body;
  if (J.tab === "quests") body = questsTab();
  else if (J.tab === "notes") body = notesTab();
  else if (!j) body = `<p class="empty-line">Opening the journal…</p>`;
  else body = ({ people: peopleTab, places: placesTab, clues: cluesTab })[J.tab](j);
  return `<div class="tabs">${JTABS.map(([k, l]) => `<button class="${J.tab === k ? "on" : ""}" data-act="jtab" data-k="${k}">${l}</button>`).join("")}</div>${find}<div id="j-body" style="display:flex;flex-direction:column;gap:22px">${body}</div>`;
}

const match = (...parts) => !J.q || parts.join(" ").toLowerCase().includes(J.q.toLowerCase());
const none = s => `<p class="empty-line">${s}</p>`;

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
  const cs = (j.chronicle || []).filter(c => match(c.text, c.kind)).slice().reverse();
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
  api("GET", `/campaigns/${S.route}/map`).then(m => { S.map = m; if (S.panel === "map") renderPanel(); }).catch(() => {});
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
    try { await addNote(text); J.draft = ""; renderPanel(); } catch (er) { toast(er.message); }
  } else if (f.dataset?.note) {
    e.preventDefault();
    const id = f.dataset.note, text = $("#n-edit").value.trim();
    if (!text) return;
    try {
      await api("POST", `/campaigns/${S.route}/notes/${id}`, { text });
      const n = J.notes.find(n => n.id === id); if (n) n.text = text;
      J.editing = ""; renderPanel();
    } catch (er) { toast(er.message); }
  }
});

document.addEventListener("click", async e => {
  const t = e.target.closest("[data-act],[data-open],[data-premise],[data-era],[data-kit],[data-prof],[data-swap],[data-step],[data-sheet],[data-dock],[data-pick],[data-check],[data-sg],[data-roll],[data-attack],[data-talk],[data-topic],[data-zone],[data-exit],[data-send]");
  if (!t || t.disabled) return;
  const ds = t.dataset, act = ds.act;
  if (ds.open) return go(`#/c/${ds.open}`);
  if (ds.premise) { $("#premise").value = ds.premise[0].toUpperCase() + ds.premise.slice(1); return; }
  if (ds.era != null && !act) { S.era = ds.era; $("#eras").outerHTML = erasHTML(); return; }
  if (ds.kit) { readEditor().kit = ds.kit; return renderBuilder(); }
  if (ds.swap) {
    const d = readEditor(), a = ds.swap;
    if (!S.swap) S.swap = a;
    else if (S.swap === a) S.swap = "";
    else { [d.scores[a], d.scores[S.swap]] = [d.scores[S.swap], d.scores[a]]; S.swap = ""; }
    return renderBuilder();
  }
  if (ds.prof) {
    const d = readEditor(), a = ds.prof, p = d.proficient || (d.proficient = []);
    if (p.includes(a)) p.splice(p.indexOf(a), 1);
    else { p.push(a); if (p.length > 2) p.shift(); }
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
    return send(o.send, o.hint);
  }
  if (ds.check) { S.hint = { check: { ability: ds.check, purpose: "other" } }; S.tray = ""; renderDock(); const ta = $("#say"); if (ta) { ta.value = S.composer = ta.value || "I try to "; ta.focus(); } return; }
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
  try {
    switch (act) {
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
          S.draft = { ...r.character, concept: r.character.concept || concept, proficient: r.character.proficient || [] };
          S.step = 3;
        } finally { renderLobby(); }
        return;
      }
      case "build": S.draft = blankDraft(); S.step = 0; return renderLobby();
      case "edit": {
        const c = mine();
        S.draft = { name: c.name, class: c.class, kit: c.kit, pronouns: c.pronouns, backstory: c.backstory || "", concept: c.concept || "", scores: { ...c.scores }, proficient: [...c.proficient] };
        S.step = 0;
        return renderLobby();
      }
      case "step-next": readEditor(); S.step = Math.min(STEPS.length - 1, S.step + 1); return renderBuilder();
      case "step-back": readEditor(); S.step = Math.max(0, S.step - 1); return renderBuilder();
      case "cancel-draft": S.draft = null; S.swap = ""; return renderLobby();
      case "save-char": {
        const d = readEditor();
        t.disabled = true;
        try {
          await api("POST", `/campaigns/${S.route}/characters`, { character: d });
          S.draft = null;
          await refreshView();
          renderLobby();
        } catch (er) { $("#d-err").textContent = er.message; t.disabled = false; }
        return;
      }
      case "start":
        if (t.classList.contains("off")) return toast("Everyone needs a character first.");
        t.disabled = true; t.innerHTML = `<span class="spinner"></span> Setting the scene…`;
        await api("POST", `/campaigns/${S.route}/start`);
        return refreshView();
      case "menu": return S.panel ? closePanel() : openPanel(S.lastPanel || "scene");
      case "panel": return S.panel === ds.k && stageMode() ? closePanel() : openPanel(ds.k);
      case "close-panel": if (S.panel === "talk") S.talkTo = S.talkTo; return closePanel();
      case "board": S.stageBoard = !S.stageBoard; return renderTop();
      case "leave-talk": S.talkTo = ""; closePanel(); renderDock(); return renderFeed();
      case "clear-check": S.hint = {}; return renderDock();
      case "skip": return api("POST", `/campaigns/${S.route}/skip`, {});
      case "jtab": J.tab = ds.k; J.q = ""; J.editing = ""; return renderPanel();
      case "note-edit": J.editing = ds.id; renderPanel(); return $("#n-edit")?.focus();
      case "note-cancel": J.editing = ""; return renderPanel();
      case "note-del":
        await api("DELETE", `/campaigns/${S.route}/notes/${ds.id}`);
        J.notes = J.notes.filter(n => n.id !== ds.id); J.editing = ""; return renderPanel();
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
