# Multi-stage build for go-DDS quickstart binaries.
# All three targets (monitor, pub, sub) are built from a single builder stage.

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
# observability/go.mod must be present before `go mod download`: the root
# go.mod's `replace github.com/SoundMatt/go-DDS/observability =>
# ./observability` directive (added in the Phase D module split,
# ROADMAP.md "Architecture Initiative" #71) resolves to this local path,
# and the Go toolchain reads it eagerly. Phase A hit the same issue for
# safety/go.mod (see #110) — this is the same fix, applied proactively.
COPY observability/go.mod observability/go.sum ./observability/
RUN go mod download
RUN cd observability && go mod download
COPY . .
# cmd/monitor moved into the observability/ submodule (ROADMAP.md
# "Architecture Initiative" #71, Phase D) — it has its own go.mod, so it
# must be built from within that module rather than as a relative package
# path off the root module.
RUN cd observability && CGO_ENABLED=0 go build -trimpath -o /bin/monitor ./cmd/monitor
RUN CGO_ENABLED=0 go build -trimpath -o /bin/pub ./examples/quickstart/pub
RUN CGO_ENABLED=0 go build -trimpath -o /bin/sub ./examples/quickstart/sub

# ── monitor image ──────────────────────────────────────────────────────────────
FROM alpine:3.21 AS monitor
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/monitor /usr/local/bin/monitor
LABEL org.opencontainers.image.title="go-DDS monitor" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-DDS" \
      org.opencontainers.image.licenses="MPL-2.0"
EXPOSE 8080
ENV MONITOR_ADDR=:8080
ENTRYPOINT ["monitor"]

# ── publisher image ────────────────────────────────────────────────────────────
FROM alpine:3.21 AS pub
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/pub /usr/local/bin/pub
LABEL org.opencontainers.image.title="go-DDS publisher" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-DDS" \
      org.opencontainers.image.licenses="MPL-2.0"
ENTRYPOINT ["pub"]

# ── subscriber image ───────────────────────────────────────────────────────────
FROM alpine:3.21 AS sub
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/sub /usr/local/bin/sub
LABEL org.opencontainers.image.title="go-DDS subscriber" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-DDS" \
      org.opencontainers.image.licenses="MPL-2.0"
ENTRYPOINT ["sub"]
