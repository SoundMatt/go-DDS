# Multi-stage build for go-DDS quickstart binaries.
# All three targets (monitor, pub, sub) are built from a single builder stage.

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/monitor ./cmd/monitor
RUN CGO_ENABLED=0 go build -trimpath -o /bin/pub     ./examples/quickstart/pub
RUN CGO_ENABLED=0 go build -trimpath -o /bin/sub     ./examples/quickstart/sub

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
