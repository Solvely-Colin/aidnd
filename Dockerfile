# The Go game server, run by Cloudflare Containers (see wrangler.jsonc).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /aidnd ./cmd/server

FROM alpine:3.22
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /aidnd /aidnd
EXPOSE 8080
ENTRYPOINT ["/aidnd"]
