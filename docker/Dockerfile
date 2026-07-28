# Multi-stage build for go-DDS quickstart binaries.
# All three targets (monitor, pub, sub) are built from a single builder stage.

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
# observability/go.mod and examples/go.mod must both be present before their
# respective `go mod download` runs below: examples/go.mod's `replace
# github.com/SoundMatt/go-DDS => ../` directive (added in the Phase E module
# split, ROADMAP.md "Architecture Initiative" #71) resolves to this root
# go.mod/go.sum (already COPY'd above), and the Go toolchain reads the
# replace target eagerly. Phase A hit the same issue for safety/go.mod (see
# #110) and Phase D proactively applied the same fix for observability/
# go.mod — this is the same fix, applied proactively again for examples/.
COPY observability/go.mod observability/go.sum ./observability/
COPY examples/go.mod examples/go.sum ./examples/
RUN go mod download
RUN cd observability && go mod download
RUN cd examples && go mod download
COPY . .
# cmd/monitor moved into the observability/ submodule (ROADMAP.md
# "Architecture Initiative" #71, Phase D) — it has its own go.mod, so it
# must be built from within that module rather than as a relative package
# path off the root module.
RUN cd observability && CGO_ENABLED=0 go build -trimpath -o /bin/monitor ./cmd/monitor
# examples/quickstart/{pub,sub} moved into the examples/ submodule
# (ROADMAP.md "Architecture Initiative" #71, Phase E) — it has its own
# go.mod, so pub/sub must be built from within that module too.
RUN cd examples && CGO_ENABLED=0 go build -trimpath -o /bin/pub ./quickstart/pub
RUN cd examples && CGO_ENABLED=0 go build -trimpath -o /bin/sub ./quickstart/sub

# ── k8s/operator builder ─────────────────────────────────────────────────────
# Separate stage: k8s/operator is its own go.mod with no `replace` back to
# the root module (ROADMAP.md, Milestone 15, "Kubernetes Operator" — it
# only talks to the Kubernetes API, not the root github.com/SoundMatt/go-DDS
# package), so it doesn't need the root go.mod/go.sum this file's main
# builder stage copies first.
FROM golang:1.26-alpine AS operator-builder
WORKDIR /src/k8s/operator
COPY k8s/operator/go.mod k8s/operator/go.sum ./
RUN go mod download
COPY k8s/operator/ .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/go-dds-operator ./cmd/operator

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

# ── operator image ─────────────────────────────────────────────────────────────
FROM alpine:3.21 AS operator
RUN apk add --no-cache ca-certificates
COPY --from=operator-builder /bin/go-dds-operator /usr/local/bin/go-dds-operator
LABEL org.opencontainers.image.title="go-DDS Kubernetes operator" \
      org.opencontainers.image.source="https://github.com/SoundMatt/go-DDS" \
      org.opencontainers.image.licenses="MPL-2.0"
USER 65532:65532
EXPOSE 9443 8081
ENTRYPOINT ["go-dds-operator"]
