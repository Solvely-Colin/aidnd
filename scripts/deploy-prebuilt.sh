#!/bin/sh
# Deploys with a Go binary built here instead of inside Docker, for
# machines where docker build has no network. CI uses the Dockerfile.
set -e
cd "$(dirname "$0")/.."
mkdir -p .local
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o .local/aidnd ./cmd/server
cat > .local/Dockerfile <<'DOCKER'
FROM alpine:3.22
COPY aidnd /aidnd
EXPOSE 8080
ENTRYPOINT ["/aidnd"]
DOCKER
sed 's#"image": "./Dockerfile"#"image": "./.local/Dockerfile"#' wrangler.jsonc > wrangler.local.jsonc
npx wrangler deploy -c wrangler.local.jsonc "$@"
