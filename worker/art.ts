// Generated art: one cover per story (a place, never the party) and one
// portrait per character. Images are made once with Workers AI and kept in
// R2. A request for an image that doesn't exist yet starts the generation
// in the background and answers 202, and the UI keeps showing its drawn
// fallback until the picture is ready.

export const ART_MODEL = "@cf/black-forest-labs/flux-2-klein-9b";
// Each image costs about 1,400 neurons, so 40 a day is about $0.60 at most.
const DEFAULT_DAILY_LIMIT = 40;
// How long a started generation blocks a second one, and how long to wait
// after a failure before trying again.
const PENDING_MS = 3 * 60_000;
const FAILED_MS = 60 * 60_000;

export type ArtEnv = {
	AI: Ai;
	ART: R2Bucket;
	ART_DAILY_LIMIT?: string;
};

// The parts of the campaign view (internal/api/views.go) the prompts use.
export type CampaignInfo = {
	title?: string;
	pitch?: string;
	premise?: string;
	party?: CharacterInfo[];
};

export type CharacterInfo = {
	id: string;
	name?: string;
	concept?: string;
	class?: string;
	pronouns?: string;
	backstory?: string;
};

export type ArtTarget = { campaign: string; kind: "cover" } | { campaign: string; kind: "char"; id: string };

const ID = /^[A-Za-z0-9_-]{1,64}$/;

// parseArtPath reads /img/<campaign>/cover and /img/<campaign>/char/<id>.
export function parseArtPath(pathname: string): ArtTarget | null {
	const parts = pathname.split("/").slice(2);
	if (parts.length === 2 && parts[1] === "cover" && ID.test(parts[0])) return { campaign: parts[0], kind: "cover" };
	if (parts.length === 3 && parts[1] === "char" && ID.test(parts[0]) && ID.test(parts[2])) {
		return { campaign: parts[0], kind: "char", id: parts[2] };
	}
	return null;
}

export function artKey(t: ArtTarget): string {
	return t.kind === "cover" ? `art/${t.campaign}/cover` : `art/${t.campaign}/char/${t.id}`;
}

const clip = (s: string | undefined, n: number) => {
	const t = (s ?? "").replace(/\s+/g, " ").trim();
	return t.length > n ? t.slice(0, n).replace(/\s\S*$/, "") + "…" : t;
};

const STYLE =
	"Painted illustration in the style of a classic tabletop fantasy adventure book, gouache and ink, " +
	"strong shapes, muted earthy palette with one warm light source. No text, no letters, no logos, no border.";

export function coverPrompt(c: CampaignInfo): string {
	return [
		"Cover art for a fantasy adventure: a wide establishing shot of the place where the story happens.",
		"No people, no characters, no figures, no faces.",
		c.title ? `Title of the adventure: ${clip(c.title, 80)}.` : "",
		`The story: ${clip(c.pitch || c.premise, 400)}`,
		STYLE,
	]
		.filter(Boolean)
		.join(" ");
}

function who(pronouns: string | undefined): string {
	const p = (pronouns ?? "").toLowerCase();
	if (p.startsWith("she")) return "a woman";
	if (p.startsWith("he")) return "a man";
	return "a person";
}

export function portraitPrompt(ch: CharacterInfo): string {
	return [
		`Head and shoulders portrait of one fantasy adventurer, ${who(ch.pronouns)}, looking toward the viewer, plain dark background.`,
		ch.class ? `Their calling: ${clip(ch.class, 60)}.` : "",
		ch.concept ? `${clip(ch.concept, 240).replace(/[^.!?…]$/, "$&.")}` : "",
		ch.backstory ? `Background: ${clip(ch.backstory, 240)}` : "",
		STYLE,
	]
		.filter(Boolean)
		.join(" ");
}

// imageType sniffs the bytes, since the model doesn't say what it returns.
export function imageType(b: Uint8Array): string {
	if (b[0] === 0x89 && b[1] === 0x50) return "image/png";
	if (b[0] === 0xff && b[1] === 0xd8) return "image/jpeg";
	if (b[0] === 0x52 && b[1] === 0x49 && b[8] === 0x57) return "image/webp";
	return "application/octet-stream";
}

async function runModel(env: ArtEnv, prompt: string, width: number, height: number): Promise<Uint8Array> {
	const form = new FormData();
	form.append("prompt", prompt);
	form.append("width", String(width));
	form.append("height", String(height));
	// The binding takes the serialized multipart body and its boundary.
	const body = new Response(form);
	const out = (await env.AI.run(ART_MODEL as any, {
		multipart: { body: body.body, contentType: body.headers.get("content-type") },
	} as any)) as { image?: string };
	if (!out?.image) throw new Error("no image in model output");
	return Uint8Array.from(atob(out.image), (c) => c.charCodeAt(0));
}

// takeQuota counts today's generations. R2 has no atomic increment, so two
// requests at the same moment can both pass; the cap is a budget guard,
// not an exact limit.
async function takeQuota(env: ArtEnv, now: number): Promise<boolean> {
	const limit = Number(env.ART_DAILY_LIMIT) || DEFAULT_DAILY_LIMIT;
	const key = `quota/${new Date(now).toISOString().slice(0, 10)}`;
	const used = Number((await env.ART.head(key))?.customMetadata?.n ?? 0);
	if (used >= limit) return false;
	await env.ART.put(key, "", { customMetadata: { n: String(used + 1) } });
	return true;
}

async function markerAge(env: ArtEnv, key: string, now: number): Promise<number | null> {
	const h = await env.ART.head(key);
	const t = Number(h?.customMetadata?.t);
	return h && t ? now - t : null;
}

// serveArt answers an image request. loadCampaign fetches the campaign view
// for the prompt; it is only called when an image has to be made.
export async function serveArt(
	t: ArtTarget,
	env: ArtEnv,
	ctx: { waitUntil(p: Promise<unknown>): void },
	loadCampaign: () => Promise<CampaignInfo | null>,
	now = Date.now(),
): Promise<Response> {
	const key = artKey(t);
	const hit = await env.ART.get(key);
	if (hit) {
		return new Response(hit.body, {
			headers: {
				"Content-Type": hit.httpMetadata?.contentType ?? "image/jpeg",
				"Cache-Control": "private, max-age=604800, immutable",
				ETag: hit.httpEtag,
			},
		});
	}
	const later = (why: string) =>
		new Response(null, { status: 202, headers: { "Cache-Control": "no-store", "X-Art": why } });

	const pending = await markerAge(env, `pending/${key}`, now);
	if (pending !== null && pending < PENDING_MS) return later("pending");
	const failed = await markerAge(env, `failed/${key}`, now);
	if (failed !== null && failed < FAILED_MS) return new Response(null, { status: 404, headers: { "X-Art": "failed" } });

	const c = await loadCampaign();
	if (!c) return new Response(null, { status: 404 });
	let prompt: string, w: number, h: number;
	if (t.kind === "cover") {
		if (!c.title && !c.pitch && !c.premise) return new Response(null, { status: 404 });
		[prompt, w, h] = [coverPrompt(c), 1024, 576];
	} else {
		const ch = c.party?.find((p) => p.id === t.id);
		if (!ch) return new Response(null, { status: 404 });
		[prompt, w, h] = [portraitPrompt(ch), 640, 640];
	}
	if (!(await takeQuota(env, now))) return new Response(null, { status: 404, headers: { "X-Art": "daily limit" } });

	await env.ART.put(`pending/${key}`, "", { customMetadata: { t: String(now) } });
	ctx.waitUntil(
		(async () => {
			try {
				const img = await runModel(env, prompt, w, h);
				await env.ART.put(key, img, { httpMetadata: { contentType: imageType(img) }, customMetadata: { prompt: prompt.slice(0, 1000) } });
			} catch (e) {
				console.error("art generation failed", key, e);
				await env.ART.put(`failed/${key}`, "", { customMetadata: { t: String(Date.now()) } });
			} finally {
				await env.ART.delete(`pending/${key}`);
			}
		})(),
	);
	return later("started");
}
