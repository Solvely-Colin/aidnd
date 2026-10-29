# aidnd

An AI game master for tabletop-style stories. The Go server owns the rules
and the world state; the model only interprets what players say and narrates
what the engine decided.

- `cmd/server`: the HTTP game server ([docs/API.md](docs/API.md))
- `internal/`: engine (`game`), model pipeline (`gm`), storage (`store`, SQLite or D1)
- `worker/` and `ui/`: the Cloudflare Worker and the player UI
- [docs/DESIGN.md](docs/DESIGN.md): rules and design decisions
- [docs/DEPLOY.md](docs/DEPLOY.md): hosting on Cloudflare

Run locally with SQLite:

```
LLM_MODEL=offline go run ./cmd/server   # open http://localhost:8080/dev/
```
