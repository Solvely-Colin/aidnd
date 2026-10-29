# Running aidnd on Cloudflare

```
browser ──> Worker (worker/index.ts) ──> GameServer container (Go, cmd/server)
              │  serves ./ui                 │ http://d1.internal  ─┐
              │  login cookie                │ http://llm.internal ─┤ handled by the Worker
              └─ D1 "aidnd" <────────────────┴──────────────────────┘
```

- The Worker serves the player UI, signs players in with a shared play code,
  and forwards `/api/*` to one container instance with `Bearer APP_SECRET`
  and `X-Player-ID` (service mode).
- The container has no internet access. Its D1 store (`internal/store/d1.go`)
  and model calls go to two virtual hosts that the Worker answers, so the
  database binding and the model key stay in the Worker.
- The container sleeps after 10 idle minutes; billing stops while it sleeps.

## One-time setup

1. Cloudflare: turn on Workers Paid. Create an API token from the
   "Edit Cloudflare Workers" template and note the account ID.
2. GitHub repo secrets: `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`,
   `APP_SECRET` (long random string), `PLAY_CODE` (what players type to sign
   in), `LLM_API_KEY` (Ollama key).
3. Push to `main`. `.github/workflows/deploy.yml` tests and runs
   `wrangler deploy`, which builds the Dockerfile and uploads the image.

Model settings are `vars` in `wrangler.jsonc`.

## Local

`npx wrangler dev` runs the Worker, a local D1 and the container (needs
Docker). Put the three secrets in `.dev.vars`. `--var LLM_MODEL:offline`
plays without model calls.
