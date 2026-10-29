// aidnd player UI. No build step: plain modules, template strings and one
// delegated click handler. Every call goes to /api/* on the Worker, which
// adds the player's id from the session cookie.

const $ = (s, el = document) => el.querySelector(s);
const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const app = $("#app");

const ICON = {
  back: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg>`,
  send: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>`,
  scene: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21s-7-6.2-7-11.5a7 7 0 0 1 14 0C19 14.8 12 21 12 21z"/><circle cx="12" cy="9.5" r="2.5"/></svg>`,
  you: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6"/></svg>`,
  book: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z"/><path d="M4 21V5M9 7h6"/></svg>`,
  close: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>`,
  menu: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="3"/><path d="M14 4v16"/></svg>`,
  heart: `<svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-8-5.2-8-11a4.5 4.5 0 0 1 8-2.8A4.5 4.5 0 0 1 20 10c0 5.8-8 11-8 11z"/></svg>`,
  chev: `<svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg>`,
  logo: `<svg class="logo" viewBox="0 0 32 32" aria-hidden="true"><path d="M16 2 29 9.5v13L16 30 3 22.5v-13z" fill="var(--accent)"/><path d="M16 8 23 20H9z" fill="var(--bg)"/></svg>`,
  quill: `<svg class="quill" viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M40 6C26 8 16 18 12 34l-2 8 6-5c14-3 22-14 24-31z"/><path d="M12 34l12-12M18 30h8"/></svg>`,
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
  sending: false,
  panel: "scene",    // scene | you | journal | map
  panelOpen: false,
  journal: null,
  map: null,
  hint: {},
  composer: "",
};

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
  Object.assign(S, { es: null, view: null, events: [], lastId: 0, draft: null, journal: null, map: null, panelOpen: false, composer: "", hint: {} });
}

// ---- sign in

function renderSignin(err = "") {
  app.innerHTML = `
  <div class="signin">
    <div class="signin-box">
      <div class="signin-head">
        ${ICON.logo}
        <h1>ai<b>dnd</b></h1>
        <p>An AI game master for you and your friends.</p>
      </div>
    <form id="signin">
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

async function renderHome() {
  app.innerHTML = `
  <div class="page">
    <header class="topbar">
      <div class="titles"><span class="brand">${ICON.logo}<span>ai<b>dnd</b></span></span></div>
      <button class="me-btn" data-act="signout" title="Sign out">Sign out<span class="avatar" style="width:30px;height:30px;font-size:13px">${esc(S.me.name[0] || "?")}</span></button>
    </header>
    <main class="wrap">
      <div class="hero"><h2>Welcome back, ${esc(S.me.name)}.</h2><p>Pick up a story, join a friend's table, or start something new.</p></div>
      <div id="lists"><div class="muted">Loading your stories…</div></div>
      <section class="section">
        <div class="section-head"><h2>Start a new story</h2></div>
        <div class="new-story" id="new">
          <textarea class="textarea serif" id="premise" rows="3" placeholder="What is this story about? A place, a mood, a mystery…"></textarea>
          <div class="chips">${PREMISES.map(p => `<button class="chip" data-premise="${esc(p)}">${esc(p)}</button>`).join("")}</div>
          <div class="foot"><span class="muted">The game master writes the world. It takes about a minute.</span>
            <button class="btn primary" data-act="create">Create story</button></div>
        </div>
      </section>
    </main>
  </div>`;
  try {
    const [mine, all] = await Promise.all([api("GET", "/me/campaigns"), api("GET", "/campaigns")]);
    if (S.route) return;
    const mineIds = new Set(mine.campaigns.map(c => c.id));
    const open = all.campaigns.filter(c => c.status === "lobby" && !mineIds.has(c.id) && c.players < c.max_players);
    const card = c => `<button class="game-card" data-open="${esc(c.id)}">
        <div class="gc-main">
          <div class="gc-top"><h3>${esc(c.title)}</h3>${statusPill(c.status)}</div>
          ${c.pitch ? `<p>${esc(c.pitch)}</p>` : ""}
          <span class="meta">${c.players}/${c.max_players} players · ${ago(c.updated_at)}</span>
        </div>${ICON.chev}
      </button>`;
    const list = (title, cs) => `<section class="section"><div class="section-head"><h2>${title}</h2></div><div class="game-list">${cs.map(card).join("")}</div></section>`;
    $("#lists").innerHTML = `
      ${mine.campaigns.length ? list("Your stories", mine.campaigns) : ""}
      ${open.length ? list("Open tables", open) : ""}
      ${!mine.campaigns.length && !open.length ? `<section class="section"><div class="empty">No stories yet. Start one below.</div></section>` : ""}`;
  } catch (e) { $("#lists").innerHTML = `<div class="error">${esc(e.message)}</div>`; }
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
  $("#new").innerHTML = `<div class="writing">${ICON.quill}<h3>Writing your world…</h3><p class="muted"><span id="elapsed">0</span>s · places, people, secrets</p></div>`;
  const t = setInterval(() => { const el = $("#elapsed"); if (el) el.textContent = Math.round((Date.now() - started) / 1000); }, 1000);
  try {
    const c = await api("POST", "/campaigns", { premise });
    await api("POST", `/campaigns/${c.id}/players`, { name: S.me.name });
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
  S.es = es;
}

let refreshTimer, feedTimer;
function onEvent(e) {
  if (e.id <= S.lastId) return;
  S.lastId = e.id;
  S.events.push(e);
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
    // Keep the editor as it is; only the party list changes.
    const p = $("#party"); if (p) p.innerHTML = partyRows();
    return;
  }
  const allReady = v.players.length > 0 && v.players.every(p => p.character_id);
  app.innerHTML = `
  <div class="page">
    <header class="topbar">
      <button class="icon-btn" data-act="home" aria-label="Back">${ICON.back}</button>
      <div class="titles"><h1>${esc(v.title)}</h1><div class="sub">Gathering the party</div></div>
    </header>
    <main class="wrap">
      <div class="story-head">
        <span class="pill accent" style="justify-self:start">New story</span>
        <h2>${esc(v.title)}</h2>
        <p class="pitch">${esc(v.pitch || v.premise)}</p>
      </div>
      <section class="section">
        <div class="section-head"><h2>Party</h2><span class="muted" style="font-size:13px">${v.players.length}/${v.max_players} seats</span></div>
        <div class="party-list" id="party">${partyRows()}</div>
        ${!v.you?.joined ? `<button class="btn primary" data-act="join">Join as ${esc(S.me.name)}</button>` : ""}
      </section>
      ${v.you?.joined ? (S.draft ? editorHTML() : me ? readyHTML(me) : conceptHTML()) : ""}
      ${v.you?.joined ? `<section class="start-bar">
        <button class="btn primary block" data-act="start" ${allReady ? "" : "disabled"}>Begin the story</button>
        <span class="muted">${allReady ? "Everyone is ready. Anyone at the table can begin." : "Starts once every player has a character. Friends can join with the same play code."}</span>
      </section>` : ""}
    </main>
  </div>`;
  wireEditor();
}

function partyRows() {
  const v = S.view;
  if (!v.players.length) return `<div class="muted">No one has joined yet.</div>`;
  return v.players.map(p => {
    const c = v.party.find(x => x.id === p.character_id);
    return `<div class="party-row"><div class="avatar">${esc((c?.name || p.name)[0] || "?")}</div>
      <div class="who"><b>${esc(c ? c.name : p.name)}</b><span>${c ? `${esc(c.class)} · played by ${esc(p.name)}` : `${esc(p.name)} is making a character…`}</span></div>
      ${c ? `<span class="pill accent">Ready</span>` : `<span class="pill">Not ready</span>`}</div>`;
  }).join("");
}

function conceptHTML() {
  const pick = CONCEPTS[Math.floor(Math.random() * CONCEPTS.length)];
  return `<section class="section">
    <div class="section-head"><h2>Your character</h2></div>
    <div class="card stack">
    <label class="field"><span>Who are you?</span>
      <textarea class="textarea serif" id="concept" rows="3" placeholder="e.g. ${esc(pick)}"></textarea></label>
    <span class="muted" style="font-size:13px">Describe them in a sentence: age, trade, a flaw, a want. The game master drafts a sheet you can change.</span>
    <button class="btn primary" data-act="draft">Draft my character</button>
    </div>
  </section>`;
}

function readyHTML(c) {
  return `<section class="section">
    <div class="section-head"><h2>Your character</h2><button class="btn small" data-act="edit">Edit</button></div>
    <div class="card stack">
    <div class="row" style="gap:14px;flex-wrap:nowrap"><div class="avatar" style="width:48px;height:48px;font-size:20px">${esc(c.name[0])}</div>
      <div><div class="serif" style="font-size:20px;font-weight:600">${esc(c.name)}</div>
      <div class="muted" style="font-size:13px">${esc(c.class)} · ${esc(c.pronouns || "they/them")} · ${esc(kitName(c.kit))}</div></div></div>
    ${c.backstory ? `<p class="serif" style="margin:0;color:var(--text-2)">${esc(c.backstory)}</p>` : ""}
    </div>
  </section>`;
}

function kitName(k) { return S.meta?.kits.find(x => x.key === k)?.name || k; }
const ABIL = ["STR", "DEX", "CON", "INT", "WIS", "CHA"];
const ABIL_NAME = { STR: "Strength", DEX: "Dexterity", CON: "Constitution", INT: "Intelligence", WIS: "Wisdom", CHA: "Charisma" };
const mod = n => Math.floor((n - 10) / 2);
const signed = n => (n >= 0 ? "+" : "") + n;

function editorHTML() {
  const d = S.draft, arr = S.meta?.standard_array || [15, 14, 13, 12, 10, 8];
  const kits = S.meta?.kits || [];
  return `<section class="section">
    <div class="section-head"><h2>Your character</h2></div>
    <div class="card stack" id="editor">
    <div class="grid2">
      <label class="field"><span>Name</span><input class="input" id="d-name" value="${esc(d.name)}" maxlength="40"></label>
      <label class="field"><span>Pronouns</span><input class="input" id="d-pron" value="${esc(d.pronouns || "they/them")}" maxlength="20"></label>
    </div>
    <label class="field"><span>Calling</span><input class="input" id="d-class" value="${esc(d.class)}" placeholder="Private eye, occultist, boxer…"></label>
    <div class="field"><span>Kit</span>
      <div class="kits">${kits.map(k => `<button class="kit ${k.key === d.kit ? "on" : ""}" data-kit="${esc(k.key)}"><b>${esc(k.name)}</b><span>${esc(k.desc)}</span></button>`).join("")}</div></div>
    <div class="field"><span>Abilities <span style="text-transform:none;letter-spacing:0;font-weight:500">· pick values, star two strengths</span></span>
      <div class="abilities">${ABIL.map(a => {
        const v = d.scores?.[a] ?? 10, prof = (d.proficient || []).includes(a);
        return `<div class="ability ${prof ? "is-prof" : ""}" title="${ABIL_NAME[a]}">
          <button class="prof" data-prof="${a}" aria-label="Proficient in ${ABIL_NAME[a]}">${prof ? "★" : "☆"}</button>
          <div class="name">${a}</div>
          <select data-ab="${a}" aria-label="${ABIL_NAME[a]}">${arr.map(x => `<option ${x === v ? "selected" : ""}>${x}</option>`).join("")}</select>
          <div class="mod">${signed(mod(v))}</div></div>`;
      }).join("")}</div></div>
    <label class="field"><span>Backstory</span><textarea class="textarea serif" id="d-back" rows="4">${esc(d.backstory)}</textarea></label>
    <div class="error" id="d-err"></div>
    <div class="row spread">
      <button class="btn ghost" data-act="cancel-draft">Cancel</button>
      <button class="btn primary" data-act="save-char">Save character</button>
    </div>
    </div>
  </section>`;
}

function readEditor() {
  const d = S.draft;
  if (!$("#editor")) return d;
  d.name = $("#d-name").value; d.pronouns = $("#d-pron").value; d.class = $("#d-class").value; d.backstory = $("#d-back").value;
  return d;
}

function wireEditor() {
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

function renderPlay() {
  const v = S.view;
  app.innerHTML = `
  <div class="play">
    <header class="topbar" id="chrome"></header>
    <div class="play-main">
      <div class="feed-scroll" id="scroll"><div class="feed" id="feed"></div></div>
      <aside class="panel" id="panel" ${S.panelOpen ? "" : "data-docked"}></aside>
    </div>
    <footer class="composer" id="composer"></footer>
  </div>
  ${S.panelOpen ? `<div class="panel-backdrop" data-act="close-panel"></div>` : ""}`;
  renderChrome();
  renderFeed(true);
}

function renderChrome() {
  const v = S.view, me = mine();
  const loc = v.location;
  const chrome = $("#chrome");
  if (!chrome) return;
  chrome.innerHTML = `
    <button class="icon-btn" data-act="home" aria-label="Back">${ICON.back}</button>
    <div class="titles"><h1>${esc(v.title)}</h1>
      <div class="sub">${esc(loc?.name || "")}${v.scene ? ` · Day ${v.scene.day}, ${esc(v.scene.time)}` : ""}${v.encounter ? ` · <span style="color:var(--bad)">Combat, round ${v.encounter.round}</span>` : ""}</div></div>
    ${me ? `<button class="hp ${me.hp <= me.max_hp / 3 ? "low" : ""}" data-panel="you" aria-label="Your character">${ICON.heart}${me.hp}/${me.max_hp}</button>` : ""}
    <button class="icon-btn hide-wide" data-panel="${S.panel}" aria-label="Scene, character, journal and map">${ICON.menu}</button>`;
  renderComposer();
  renderPanel();
}

// feed

function nearBottom() { const s = $("#scroll"); return !s || s.scrollHeight - s.scrollTop - s.clientHeight < 160; }
function toBottom() { const s = $("#scroll"); if (s) s.scrollTop = s.scrollHeight; }

function renderFeed(force) {
  const feed = $("#feed");
  if (!feed) { if (S.view?.status === "lobby") renderSetup(true); return; }
  const stick = force || nearBottom();
  feed.innerHTML = S.events.map(eventHTML).join("") + (isThinking() ? `<div class="thinking"><span class="dots"><i></i><i></i><i></i></span>The story unfolds…</div>` : "");
  if (stick) requestAnimationFrame(toBottom);
}

// isThinking: an action is in flight and its narration has not arrived.
function isThinking() {
  if (S.sending) return true;
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
    return `<span class="roll" title="${esc(r.expr)} [${(r.dice || []).join(", ")}]"><span class="die">${r.total}</span>${esc(r.label || r.kind)}</span>`;
  }
  const crit = tier === "critical_success" ? " · critical!" : tier === "critical_failure" ? " · fumble" : "";
  return `<span class="roll ${esc(tier)}" title="d20 ${r.natural} ${signed(r.mod)} = ${r.total} vs ${r.target}"><span class="die">${r.total}</span>${esc(r.label)} <span class="muted">vs ${r.target}${crit}</span></span>`;
}

function eventHTML(e) {
  const d = e.data || {}, me = mine();
  switch (e.type) {
    case "player_action": {
      const you = me && d.character_id === me.id;
      return `<div class="ev-you ${you ? "" : "ev-other"}"><div class="bubble"><span class="by">${esc(you ? "You" : d.character)}</span>${esc(d.text)}</div></div>`;
    }
    case "action_resolved": {
      let h = "";
      const rolls = (d.steps || []).flatMap(s => s.rolls || []).map(rollHTML).join("");
      if (rolls) h += `<div class="rolls">${rolls}</div>`;
      const notes = [];
      (d.steps || []).forEach(s => { if (s.error) notes.push(`<div class="ev-note bad"><span class="ic">✕</span>${esc(s.error)}</div>`); });
      (d.skipped || []).forEach(s => notes.push(`<div class="ev-note"><span class="ic">⤼</span>Not this turn: ${esc(s.error)}</div>`));
      const changes = (d.steps || []).flatMap(s => s.changes || []).concat(d.world_changes || []);
      if (changes.length) h += `<div class="changes">${changes.map(c => `<div>${esc(c)}</div>`).join("")}</div>`;
      return h + notes.join("");
    }
    case "narration":
      if (d.skip && !d.text) return "";
      return `<p class="ev-narr ${d.quick ? "quick" : ""} ${d.opening ? "opening" : ""}">${esc(d.text)}</p>`;
    case "enemy_turns": {
      const rolls = (d.turns || []).map(t => t.attack?.roll ? rollHTML({ ...t.attack.roll, label: t.attack.roll.label || t.enemy }) : "").join("");
      return `${rolls ? `<div class="rolls">${rolls}</div>` : ""}${d.narration ? `<p class="ev-narr">${esc(d.narration)}</p>` : ""}`;
    }
    case "quest_updated": return `<div class="ev-note quest"><span class="ic">✦</span>${esc(d.text || d.title)}</div>`;
    case "location_changed": return `<div class="ev-sys">${esc(d.name)}</div>`;
    case "turn_started": return me && d.character_id === me.id ? `<div class="ev-sys">Your turn</div>` : `<div class="ev-sys">${esc(d.name)}'s turn</div>`;
    case "campaign_ended": return `<div class="ev-sys">${d.status === "completed" ? "The story is complete" : "The story ends in failure"}</div>`;
    case "character_created": return `<div class="ev-note"><span class="ic">+</span>${esc(d.name)} the ${esc(d.class)} joins the party</div>`;
    default: return "";
  }
}

// composer

function suggestions() {
  const v = S.view, loc = v.location || {}, me = mine(), out = [];
  if (!v.you?.your_turn) return out;
  for (const e of v.encounter?.enemies || []) if (e.hp > 0) out.push({ label: `Attack ${e.display}`, text: `I attack ${e.display}`, hint: { target_id: e.id } });
  if (!v.encounter) out.push({ label: "Look around", text: "I look around carefully" });
  for (const n of loc.npcs || []) if (n.alive !== false && !v.encounter) out.push({ label: `Talk to ${n.name}`, text: `I talk to ${n.name}: `, hint: { target_id: n.id } });
  for (const i of loc.items || []) out.push({ label: `Take ${i.name}`, text: `I pick up the ${i.name}` });
  for (const x of loc.exits || []) if (x.state === "open") out.push({ label: `Go to ${x.to_name}`, text: v.encounter ? `I flee toward ${x.to_name}` : `I travel to ${x.to_name}` });
  if (me && me.hp < me.max_hp && !v.encounter) out.push({ label: "Rest", text: "I rest for a while" });
  return out.slice(0, 10);
}

function renderComposer() {
  const v = S.view, el = $("#composer");
  if (!el) return;
  const ended = v.status === "completed" || v.status === "failed";
  if (ended) {
    el.innerHTML = `<div class="composer-inner"><div class="turnline"><span class="grow">${v.status === "completed" ? "The story is complete." : "The story has ended."}</span><button class="btn small" data-act="home">Back home</button></div></div>`;
    return;
  }
  const yours = !!v.you?.your_turn, b = v.budget || {};
  const slot = (name, n) => `<span class="slot ${n === 0 ? "spent" : ""}">${name}${n > 1 ? " " + n : ""}</span>`;
  const idle = v.turn_started ? Date.now() / 1000 - v.turn_started : 0;
  const turn = yours
    ? `<span class="you">Your turn</span><span class="grow"></span><span class="budget">${slot("Action", b.action)}${slot("Move", b.move)}${b.free === -1 ? "" : slot("Free", b.free)}</span>`
    : `<span class="grow">${v.active ? `Waiting for <b>${esc(v.active.name)}</b>…` : ""}</span>${v.you?.joined && idle > 120 ? `<button class="btn small ghost" data-act="skip">Skip their turn</button>` : ""}`;
  const sug = suggestions();
  const kept = $("#say")?.value ?? S.composer;
  el.innerHTML = `<div class="composer-inner">
    <div class="turnline">${turn}</div>
    ${sug.length ? `<div class="suggest">${sug.map((s, i) => `<button class="chip" data-sug="${i}">${esc(s.label)}</button>`).join("")}${yours ? `<button class="chip" data-act="end-turn">End turn</button>` : ""}</div>` : ""}
    <form class="say" id="say-form">
      <textarea id="say" rows="1" placeholder="${yours ? "What do you do?" : "Wait for your turn…"}" ${yours && !S.sending ? "" : "disabled"} enterkeyhint="send">${esc(kept)}</textarea>
      <button class="send" aria-label="Send" ${yours && !S.sending ? "" : "disabled"}>${S.sending ? `<span class="spinner"></span>` : ICON.send}</button>
    </form></div>`;
  el._sug = sug;
  const ta = $("#say");
  autosize(ta);
  ta.oninput = () => { S.composer = ta.value; autosize(ta); };
  ta.onkeydown = e => { if (e.key === "Enter" && !e.shiftKey && !matchMedia("(pointer: coarse)").matches) { e.preventDefault(); sendAction(); } };
  $("#say-form").onsubmit = e => { e.preventDefault(); sendAction(); };
}
function autosize(ta) { ta.style.height = "auto"; ta.style.height = Math.min(ta.scrollHeight, 140) + "px"; }

function fill(text, hint = {}) {
  const ta = $("#say");
  if (!ta || ta.disabled) return toast("It is not your turn yet.");
  S.hint = hint;
  ta.value = text; S.composer = text; autosize(ta);
  if (S.panelOpen && innerWidth < 980) { S.panelOpen = false; renderPlay(); }
  const t = $("#say"); t.focus(); t.setSelectionRange(t.value.length, t.value.length);
}

async function sendAction() {
  const ta = $("#say"), text = ta?.value.trim();
  if (!text || S.sending) return;
  const hint = S.hint; S.hint = {};
  S.sending = true; S.composer = ""; ta.value = "";
  renderComposer(); renderFeed(true);
  try {
    await api("POST", `/campaigns/${S.route}/actions`, { text, ...hint });
  } catch (e) {
    toast(e.message);
    S.composer = text;
  } finally {
    S.sending = false;
    await refreshView();
    renderFeed();
  }
}

// side panel

function renderPanel() {
  const el = $("#panel");
  if (!el) return;
  const tabs = [["scene", "Scene"], ["you", "You"], ["journal", "Journal"], ["map", "Map"]];
  el.innerHTML = `<div class="panel-head"><div class="tabs">${tabs.map(([k, l]) => `<button class="tab ${S.panel === k ? "on" : ""}" data-panel="${k}">${l}</button>`).join("")}</div>
    <button class="icon-btn close" data-act="close-panel" aria-label="Close">${ICON.close}</button></div>
    <div class="panel-body">${({ scene: sceneHTML, you: youHTML, journal: journalHTML, map: mapHTML })[S.panel]()}</div>`;
}

function hpbar(hp, max) { const p = Math.max(0, Math.min(1, hp / max)); return `<div class="hpbar ${p <= .34 ? "low" : ""}"><i style="width:${p * 100}%"></i></div>`; }

function sceneHTML() {
  const v = S.view, l = v.location || {};
  const zname = id => l.zones?.find(z => z.id === id)?.name || "";
  return `
    <section><h3>${esc(l.name)}</h3><p class="serif" style="margin:0;color:var(--text-2)">${esc(l.description)}</p></section>
    ${l.zones?.length > 1 ? `<section>${zoneMap()}</section>` : ""}
    ${v.encounter ? `<section><p class="eyebrow">Enemies · round ${v.encounter.round}</p><div class="list">${v.encounter.enemies.map(e => `
      <button class="item" data-fill="I attack ${esc(e.display)}" data-target="${esc(e.id)}" ${e.hp <= 0 ? "disabled" : ""}>
        <div class="main"><b>${esc(e.display)}</b><span>${e.hp > 0 ? `${e.hp}/${e.max_hp} HP${e.ranged ? " · ranged" : ""} · ${esc(zname(e.zone_id))}` : "Defeated"}</span>${e.hp > 0 ? hpbar(e.hp, e.max_hp) : ""}</div></button>`).join("")}</div></section>` : ""}
    ${l.npcs?.length ? `<section><p class="eyebrow">People here</p><div class="list">${l.npcs.map(n => `
      <button class="item" data-fill="I talk to ${esc(n.name)}: " data-target="${esc(n.id)}"><div class="avatar">${esc(n.name[0])}</div>
        <div class="main"><b>${esc(n.name)}</b><span>${esc(n.role)}${n.merchant ? " · sells goods" : ""}</span></div></button>`).join("")}</div></section>` : ""}
    ${l.items?.length ? `<section><p class="eyebrow">You notice</p><div class="list">${l.items.map(i => `
      <button class="item" data-fill="I pick up the ${esc(i.name)}"><div class="main"><b>${esc(i.name)}</b>${i.effect ? `<span>${esc(i.effect)}</span>` : ""}</div></button>`).join("")}</div></section>` : ""}
    ${l.exits?.length ? `<section><p class="eyebrow">Ways out</p><div class="list">${l.exits.map(x => `
      <button class="item" data-fill="${x.state === "open" ? `I travel to ${esc(x.to_name)}` : `I try the ${esc(x.label)}`}"><div class="main"><b>${esc(x.to_name)}</b><span>${esc(x.label)}${x.state === "locked" ? " · locked" : ""}${x.explored ? "" : " · unexplored"}</span></div></button>`).join("")}</div></section>` : ""}
    <section><p class="eyebrow">Party</p><div class="list">${v.party.map(c => `
      <div class="item"><div class="avatar">${esc(c.name[0])}</div><div class="main"><b>${esc(c.name)}${v.active?.character_id === c.id ? ` <span class="pill accent" style="margin-left:4px">acting</span>` : ""}</b>
        <span>${esc(c.class)} · ${c.hp}/${c.max_hp} HP${c.down ? " · down" : ""}${zname(c.zone_id) ? " · " + esc(zname(c.zone_id)) : ""}</span>${hpbar(c.hp, c.max_hp)}</div></div>`).join("")}</div></section>`;
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
  v.party.forEach(c => add(c.zone_id, { fill: c.id === me?.id ? "var(--good)" : "var(--info)", name: c.name, faded: c.down }));
  (loc.npcs || []).forEach(n => add(n.zone_id, { fill: "var(--text-3)", name: n.name }));
  (v.encounter?.enemies || []).filter(e => e.hp > 0).forEach(e => add(e.zone_id, { fill: "var(--bad)", name: e.display }));
  zones.forEach(z => {
    const p = pos[z.id], short = z.name.length > 15 ? z.name.slice(0, 14) + "…" : z.name;
    s += `<g data-fill="I move to ${esc(z.name)}" data-zone="${esc(z.id)}" style="cursor:pointer"><rect class="zone ${z.id === me?.zone_id ? "mine" : ""}" x="${p.x}" y="${p.y}" width="${W}" height="${H}" rx="10"/>
      <title>${esc(z.name)}${z.desc ? ": " + esc(z.desc) : ""}</title><text x="${p.x + 8}" y="${p.y + 16}">${esc(short)}${z.cover ? " ◆" : ""}</text>`;
    (tokens[z.id] || []).forEach((t, i) => {
      const tx = p.x + 16 + (i % 4) * 22, ty = p.y + 38 + Math.floor(i / 4) * 20;
      s += `<g class="tok"><title>${esc(t.name)}</title><circle cx="${tx}" cy="${ty}" r="9" fill="${t.fill}" opacity="${t.faded ? .35 : 1}"/><text x="${tx}" y="${ty + 4}" text-anchor="middle">${esc(t.name[0])}</text></g>`;
    });
    s += `</g>`;
  });
  return s + `</svg><div class="muted" style="font-size:12px">Green is you, blue are allies, red are enemies. Tap a zone to move.</div>`;
}

function youHTML() {
  const c = mine();
  if (!c) return `<div class="muted">You are watching this story.</div>`;
  return `
    <section><div class="row" style="gap:14px"><div class="avatar" style="width:52px;height:52px;font-size:22px">${esc(c.name[0])}</div>
      <div><h3>${esc(c.name)}</h3><div class="muted" style="font-size:13px">${esc(c.class)} · level ${c.level} · ${esc(c.pronouns || "they/them")}</div></div></div></section>
    <section><div class="row spread" style="font-size:13px"><span>Health ${c.hp}/${c.max_hp}${c.down ? " · down" : ""}</span><span class="muted">XP ${c.xp}${c.next_level_xp ? " / " + c.next_level_xp : ""}</span></div>${hpbar(c.hp, c.max_hp)}</section>
    <section class="statgrid">
      <div class="stat"><div class="k">ARMOR</div><div class="v">${c.ac}</div></div>
      <div class="stat"><div class="k">GOLD</div><div class="v">${c.gold}</div></div>
      <div class="stat"><div class="k">PROF</div><div class="v">${signed(c.proficiency_bonus)}</div></div>
    </section>
    <section class="statgrid">${ABIL.map(a => `<div class="stat ${c.proficient.includes(a) ? "prof" : ""}" title="${ABIL_NAME[a]}"><div class="k">${a}${c.proficient.includes(a) ? " ★" : ""}</div><div class="v">${c.scores[a]}</div><div class="m">${signed(c.modifiers[a])}</div></div>`).join("")}</section>
    ${c.weapon ? `<section><p class="eyebrow">Weapon</p><div class="item"><div class="main"><b>${esc(c.weapon.name)}</b><span>${esc(c.weapon.damage)} · ${esc(c.weapon.reach)}</span></div></div></section>` : ""}
    <section><p class="eyebrow">Carrying</p><div class="list">${c.inventory.map(i => `
      <button class="item" data-fill="${i.equipped ? `I use the ${esc(i.name)}` : `I ${i.kind === "consumable" ? "use" : "equip"} the ${esc(i.name)}`}"><div class="main"><b>${esc(i.name)}${i.equipped ? ` <span class="pill" style="margin-left:4px">equipped</span>` : ""}</b>${i.effect ? `<span>${esc(i.effect)}</span>` : ""}</div></button>`).join("") || `<div class="muted">Nothing.</div>`}</div></section>
    ${c.backstory ? `<section><p class="eyebrow">Backstory</p><p class="serif" style="margin:0;color:var(--text-2)">${esc(c.backstory)}</p></section>` : ""}`;
}

function journalHTML() {
  const v = S.view;
  if (!S.journal) {
    api("GET", `/campaigns/${S.route}/journal`).then(j => { S.journal = j; if (S.panel === "journal") renderPanel(); }).catch(() => {});
  }
  const j = S.journal;
  const quests = v.quests.filter(q => q.status !== "available" || q.is_main);
  return `
    <section><p class="eyebrow">Quests</p>${quests.length ? quests.map(q => `
      <div class="quest-card"><div class="row spread"><b class="serif" style="font-size:16px">${esc(q.title)}</b>${q.is_main ? `<span class="pill accent">Main</span>` : `<span class="pill">${esc(q.status)}</span>`}</div>
        ${q.summary ? `<div class="muted" style="font-size:13px">${esc(q.summary)}</div>` : ""}
        ${q.objectives.map(o => `<div class="objective ${o.done ? "done" : ""}"><span>${o.done ? "☑" : "☐"}</span><span>${esc(o.text)}</span></div>`).join("")}
        ${q.reward_gold || q.reward_xp ? `<div class="muted" style="font-size:12px">Reward: ${[q.reward_gold && q.reward_gold + " gold", q.reward_xp && q.reward_xp + " XP"].filter(Boolean).join(", ")}${q.giver_name ? " · from " + esc(q.giver_name) : ""}</div>` : ""}
      </div>`).join("") : `<div class="muted">No quests yet.</div>`}</section>
    ${j ? `${j.summary ? `<section><p class="eyebrow">The story so far</p><p class="serif" style="margin:0;color:var(--text-2)">${esc(j.summary)}</p></section>` : ""}
      <section><p class="eyebrow">Chronicle</p>${j.chronicle.length ? j.chronicle.slice().reverse().map(c => `<div class="ev-note"><span class="ic">·</span>${esc(c.text)}</div>`).join("") : `<div class="muted">Nothing recorded yet.</div>`}</section>`
    : `<div class="muted">Loading the journal…</div>`}`;
}

function mapHTML() {
  if (!S.map) {
    api("GET", `/campaigns/${S.route}/map`).then(m => { S.map = m; if (S.panel === "map") renderPanel(); }).catch(() => {});
    return `<div class="muted">Unfolding the map…</div>`;
  }
  const m = S.map, here = m.current_id;
  const byId = Object.fromEntries(m.locations.map(l => [l.id, l]));
  const near = new Map();
  m.edges.forEach(e => { if (e.a === here) near.set(e.b, e.state); if (e.b === here) near.set(e.a, e.state); });
  const others = m.locations.filter(l => l.id !== here && !near.has(l.id));
  const row = (l, extra = "", fillText = "") => `<${fillText ? `button data-fill="${esc(fillText)}"` : "div"} class="item"><div class="main"><b>${esc(l.name)}${l.id === m.hub_id ? ` <span class="pill" style="margin-left:4px">home</span>` : ""}</b><span>${esc(l.kind || "")}${extra}</span></div></${fillText ? "button" : "div"}>`;
  return `
    <section><p class="eyebrow">You are here</p>${byId[here] ? row(byId[here]) : ""}</section>
    ${near.size ? `<section><p class="eyebrow">Paths from here</p><div class="list">${[...near].map(([id, st]) => byId[id] ? row(byId[id], st === "locked" ? " · locked" : byId[id].visited ? " · visited" : " · not yet visited", st === "locked" ? "" : `I travel to ${byId[id].name}`) : "").join("")}</div></section>` : ""}
    ${others.length ? `<section><p class="eyebrow">Known places</p><div class="list">${others.map(l => row(l, l.visited ? " · visited" : "")).join("")}</div></section>` : ""}`;
}

// ---- actions

document.addEventListener("click", async e => {
  const t = e.target.closest("[data-act],[data-open],[data-premise],[data-kit],[data-prof],[data-panel],[data-sug],[data-fill]");
  if (!t) return;
  const act = t.dataset.act;
  if (t.dataset.open) return go(`#/c/${t.dataset.open}`);
  if (t.dataset.premise) { $("#premise").value = t.dataset.premise; return; }
  if (t.dataset.kit) { readEditor().kit = t.dataset.kit; return renderSetup(); }
  if (t.dataset.prof) {
    const d = readEditor(), a = t.dataset.prof, p = d.proficient || (d.proficient = []);
    if (p.includes(a)) p.splice(p.indexOf(a), 1);
    else { p.push(a); if (p.length > 2) p.shift(); }
    return renderSetup();
  }
  if (t.dataset.panel) {
    const wide = innerWidth >= 980;
    if (!wide && S.panelOpen && S.panel === t.dataset.panel && !t.classList.contains("tab")) { S.panelOpen = false; return renderPlay(); }
    S.panel = t.dataset.panel;
    if (!wide && !S.panelOpen) { S.panelOpen = true; return renderPlay(); }
    return renderPanel();
  }
  if (t.dataset.sug) { const s = $("#composer")._sug[+t.dataset.sug]; return fill(s.text, s.hint); }
  if (t.dataset.fill) {
    const hint = t.dataset.target ? { target_id: t.dataset.target } : t.dataset.zone ? { zone_id: t.dataset.zone } : {};
    return fill(t.dataset.fill, hint);
  }
  try {
    switch (act) {
      case "signout": await fetch("/auth/logout", { method: "POST" }); S.me = null; leaveCampaign(); S.route = ""; location.hash = ""; return renderSignin();
      case "home": return go("#/");
      case "create": return createCampaign();
      case "close-panel": S.panelOpen = false; return renderPlay();
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
        } finally { renderSetup(); }
        return;
      }
      case "edit": {
        const c = mine();
        S.draft = { name: c.name, class: c.class, kit: c.kit, pronouns: c.pronouns, backstory: c.backstory || "", concept: c.concept || "", scores: { ...c.scores }, proficient: [...c.proficient] };
        return renderSetup();
      }
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
