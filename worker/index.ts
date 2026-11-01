// The aidnd Worker. It serves the player UI (static assets in ./ui), signs
// players in with a shared play code, and forwards /api/* to the Go game
// server running in a Cloudflare Container. The container has no internet
// access of its own: it reaches D1 and the model only through the two
// bridges below, which run here in the Workers runtime and hold the keys.

import { Container, ContainerProxy, getContainer } from "@cloudflare/containers";
import { parseArtPath, serveArt, type CampaignInfo } from "./art";

export { ContainerProxy };

export interface Env {
	ASSETS: Fetcher;
	GAME: DurableObjectNamespace<GameServer>;
	DB: D1Database;
	AI: Ai;
	ART: R2Bucket;
	ART_DAILY_LIMIT?: string;
	LLM_MODEL: string;
	LLM_BASE_URL: string;
	LLM_REASONING_EFFORT: string;
	// Player ids that may use the GM area, comma separated ("colin").
	GM_PLAYERS?: string;
	// Secrets.
	APP_SECRET: string;
	PLAY_CODE: string;
	LLM_API_KEY: string;
	// Optional. When set, a GM name signs in with this code instead of the
	// play code, so nobody else can sign in under the GM's name.
	GM_CODE?: string;
}

// --- bridges for the container

type Stmt = { sql: string; params?: unknown[] };
type D1Req = { mode: "query"; sql: string; params?: unknown[] } | { mode: "batch"; stmts: Stmt[] };

// d1Bridge speaks the wire format of internal/store/d1.go.
export async function d1Bridge(req: Request, env: Env): Promise<Response> {
	if (req.method !== "POST") return Response.json({ error: "POST only" }, { status: 405 });
	let body: D1Req;
	try {
		body = await req.json();
	} catch {
		return Response.json({ error: "bad json" }, { status: 400 });
	}
	try {
		if (body.mode === "query") {
			const rows = await env.DB.prepare(body.sql)
				.bind(...(body.params ?? []))
				.raw<unknown[]>({ columnNames: true });
			const [columns = [], ...rest] = rows;
			return Response.json({ columns, rows: rest });
		}
		if (body.mode === "batch") {
			if (!body.stmts?.length) return Response.json({ results: [] });
			const out = await env.DB.batch(body.stmts.map((s) => env.DB.prepare(s.sql).bind(...(s.params ?? []))));
			return Response.json({
				results: out.map((r) => ({ changes: r.meta?.changes ?? 0, last_row_id: r.meta?.last_row_id ?? 0 })),
			});
		}
		return Response.json({ error: "unknown mode" }, { status: 400 });
	} catch (e) {
		// SQL errors are the caller's; the Go driver does not retry a 400.
		return Response.json({ error: String((e as Error)?.message ?? e) }, { status: 400 });
	}
}

// llmBridge forwards model calls to LLM_BASE_URL with the real key. The
// container uses http://llm.internal/v1 as its base URL, so the path
// carries over unchanged.
export async function llmBridge(req: Request, env: Env): Promise<Response> {
	const u = new URL(req.url);
	const target = new URL(env.LLM_BASE_URL).origin + u.pathname + u.search;
	const headers = new Headers(req.headers);
	headers.set("Authorization", `Bearer ${env.LLM_API_KEY}`);
	headers.delete("Host");
	return fetch(target, { method: req.method, headers, body: req.body });
}

// --- the game server container

export class GameServer extends Container<Env> {
	defaultPort = 8080;
	// Sleeps after 10 idle minutes so billing stops; waking takes a few seconds.
	sleepAfter = "10m";
	enableInternet = false;

	constructor(ctx: DurableObjectState<{}>, env: Env) {
		super(ctx, env);
		// The durable_object scheduling policy picks the image and size at
		// start time; @cloudflare/containers doesn't pass them, so add them.
		const container = ctx.container as any;
		(this as any).container = new Proxy(container, {
			get(target, prop) {
				if (prop === "start") {
					return (opts: object) =>
						target.start({ ...opts, image: target.images.gameserver, instance: "lite" });
				}
				const value = target[prop];
				return typeof value === "function" ? value.bind(target) : value;
			},
		});
		this.envVars = {
			ADDR: ":8080",
			API_KEY: env.APP_SECRET,
			DEV_UI: "false",
			D1_URL: "http://d1.internal/",
			LLM_MODEL: env.LLM_MODEL,
			LLM_BASE_URL: "http://llm.internal/v1",
			// The bridge replaces this with the real key.
			LLM_API_KEY: "bridge",
			LLM_REASONING_EFFORT: env.LLM_REASONING_EFFORT ?? "",
		};
		// A deploy restarts this object but not its container: with the
		// durable_object policy a running container keeps its old image, and
		// an open event stream keeps it awake, so the game server could stay
		// on old code for hours. Every deploy runs this constructor, so here
		// is where a container on an old image is stopped. The next request
		// starts it on the new one; saved games live in D1.
		ctx.blockConcurrencyWhile(() => this.upgradeImage());
	}

	async upgradeImage(): Promise<void> {
		try {
			const c = this.ctx.container as any;
			const want: string | undefined = c?.images?.gameserver;
			if (!c?.running || !want) return;
			const info = await c.inspect();
			// inspect reports an empty image while starting; leave those be.
			if (!info?.image || info.image === want) return;
			console.log(`game server on ${info.image}, deploying ${want}: restarting`);
			// SIGTERM lets the server finish in-flight requests (10 s).
			await this.stop();
			for (let i = 0; i < 24 && c.running; i++) await new Promise((r) => setTimeout(r, 500));
			if (c.running) await this.destroy();
		} catch (e) {
			console.log("image check failed:", (e as Error)?.message ?? e);
		}
	}
}

// Assigned, not declared as a static field: the setter registers the
// handlers, and a class field would shadow it.
GameServer.outboundByHost = {
	"d1.internal": (req: Request, env: Env) => d1Bridge(req, env),
	"llm.internal": (req: Request, env: Env) => llmBridge(req, env),
};

// --- sessions

const COOKIE = "aidnd_session";
const SESSION_DAYS = 60;
const enc = new TextEncoder();

// g marks a session signed in with the GM code.
type Session = { p: string; n: string; e: number; g?: 1 };

function b64url(buf: ArrayBuffer | Uint8Array): string {
	const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
	let s = "";
	for (const b of bytes) s += String.fromCharCode(b);
	return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function unb64url(s: string): Uint8Array {
	const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
	return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

async function hmac(secret: string, msg: string): Promise<Uint8Array> {
	const key = await crypto.subtle.importKey("raw", enc.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
	return new Uint8Array(await crypto.subtle.sign("HMAC", key, enc.encode(msg)));
}

function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
	if (a.length !== b.length) return false;
	let d = 0;
	for (let i = 0; i < a.length; i++) d |= a[i] ^ b[i];
	return d === 0;
}

export async function signSession(secret: string, s: Session): Promise<string> {
	const payload = b64url(enc.encode(JSON.stringify(s)));
	return payload + "." + b64url(await hmac(secret, "session:" + payload));
}

export async function readSession(secret: string, cookieHeader: string | null, now = Date.now()): Promise<Session | null> {
	const raw = (cookieHeader ?? "")
		.split(/;\s*/)
		.find((c) => c.startsWith(COOKIE + "="))
		?.slice(COOKIE.length + 1);
	if (!raw) return null;
	const [payload, sig] = raw.split(".");
	if (!payload || !sig) return null;
	try {
		if (!sameBytes(unb64url(sig), await hmac(secret, "session:" + payload))) return null;
		const s = JSON.parse(new TextDecoder().decode(unb64url(payload))) as Session;
		return s.e > now && s.p ? s : null;
	} catch {
		return null;
	}
}

function gmIds(env: Env): string[] {
	return (env.GM_PLAYERS ?? "colin").split(",").map((x) => playerId(x)).filter(Boolean);
}

// isGM: a GM name, and when a GM code is set, signed in with it.
export function isGM(env: Env, s: Session | null): boolean {
	if (!s || !gmIds(env).includes(s.p)) return false;
	return !env.GM_CODE || s.g === 1;
}

async function codeIs(env: Env, given: string, want: string | undefined): Promise<boolean> {
	if (!want) return false;
	// Compare HMACs so the comparison takes the same time for any guess.
	return sameBytes(await hmac(env.APP_SECRET, "code:" + given.trim()), await hmac(env.APP_SECRET, "code:" + want));
}

// playerId turns a display name into a stable id: "Colin" -> "colin".
export function playerId(name: string): string {
	return name
		.toLowerCase()
		.normalize("NFKD")
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "")
		.slice(0, 40);
}

function cookie(value: string, maxAge: number): string {
	return `${COOKIE}=${value}; Path=/; HttpOnly; Secure; SameSite=Lax; Max-Age=${maxAge}`;
}

async function login(req: Request, env: Env): Promise<Response> {
	let body: { name?: string; code?: string };
	try {
		body = await req.json();
	} catch {
		return Response.json({ error: "bad json" }, { status: 400 });
	}
	const name = (body.name ?? "").trim().slice(0, 40);
	const id = playerId(name);
	if (!id) return Response.json({ error: "Enter your name." }, { status: 400 });
	const code = body.code ?? "";
	const s: Session = { p: id, n: name, e: Date.now() + SESSION_DAYS * 86400_000 };
	if (env.GM_CODE && gmIds(env).includes(id)) {
		// the GM's name is held by the GM code
		if (!(await codeIs(env, code, env.GM_CODE))) {
			return Response.json({ error: "That name belongs to the GM. Sign in with the GM code, or pick another name." }, { status: 401 });
		}
		s.g = 1;
	} else if (!(await codeIs(env, code, env.PLAY_CODE))) {
		return Response.json({ error: "That play code is not right." }, { status: 401 });
	}
	return Response.json(
		{ player_id: id, name, gm: isGM(env, s) },
		{ headers: { "Set-Cookie": cookie(await signSession(env.APP_SECRET, s), SESSION_DAYS * 86400) } },
	);
}

// --- routing

export async function proxyToGame(req: Request, env: Env, s: Session): Promise<Response> {
	const u = new URL(req.url);
	u.pathname = u.pathname.replace(/^\/api/, "") || "/";
	const headers = new Headers(req.headers);
	headers.delete("Cookie");
	headers.set("Authorization", `Bearer ${env.APP_SECRET}`);
	headers.set("X-Player-ID", s.p);
	// only the Worker says who the GM is
	headers.delete("X-GM");
	if (isGM(env, s)) headers.set("X-GM", "1");
	const fwd = new Request(u.toString(), { method: req.method, headers, body: req.body, redirect: "manual" });
	return getContainer(env.GAME, "main").fetch(fwd);
}

// loadCampaign reads a campaign view for an art prompt.
async function loadCampaign(req: Request, env: Env, s: Session, id: string): Promise<CampaignInfo | null> {
	const u = new URL(req.url);
	const r = await getContainer(env.GAME, "main").fetch(
		new Request(`${u.origin}/campaigns/${encodeURIComponent(id)}`, {
			headers: { Authorization: `Bearer ${env.APP_SECRET}`, "X-Player-ID": s.p },
		}),
	);
	return r.ok ? ((await r.json()) as CampaignInfo) : null;
}

export default {
	async fetch(req: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
		const { pathname } = new URL(req.url);
		if (pathname === "/auth/login" && req.method === "POST") return login(req, env);
		if (pathname === "/auth/logout" && req.method === "POST") {
			return Response.json({ ok: true }, { headers: { "Set-Cookie": cookie("", 0) } });
		}
		const s = await readSession(env.APP_SECRET, req.headers.get("Cookie"));
		if (pathname === "/auth/me") {
			return s ? Response.json({ player_id: s.p, name: s.n, gm: isGM(env, s) }) : Response.json({ error: "signed out" }, { status: 401 });
		}
		if (pathname.startsWith("/api/")) {
			if (!s) return Response.json({ error: "Sign in first." }, { status: 401 });
			return proxyToGame(req, env, s);
		}
		if (pathname.startsWith("/img/")) {
			if (!s) return new Response(null, { status: 401 });
			const t = parseArtPath(pathname);
			if (!t) return new Response(null, { status: 404 });
			return serveArt(t, env, ctx, () => loadCampaign(req, env, s, t.campaign));
		}
		if (pathname.startsWith("/auth/")) return Response.json({ error: "not found" }, { status: 404 });
		return env.ASSETS.fetch(req);
	},
	// The cron wakes the game server to run its timers.
	async scheduled(_ev: ScheduledController, env: Env, ctx: ExecutionContext): Promise<void> {
		const tick = getContainer(env.GAME, "main").fetch(
			new Request("http://game/tick", { method: "POST", headers: { Authorization: `Bearer ${env.APP_SECRET}` } }),
		);
		ctx.waitUntil(tick.then(r => r.text()).catch(() => undefined));
	},
} satisfies ExportedHandler<Env>;
