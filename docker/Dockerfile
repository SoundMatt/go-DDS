# Multi-stage build for go-DDS quickstart binaries.
# All three targets (monitor, pub, sub) are built from a single builder stage.

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/monitor ./cmd/monitor
RUN CGO_ENABLED=0 go build -trimpath -o /bin/pub     ./examples/quickstart/pub
RUN CGO_ENABLED=0 go build -trimpath -o /bin/sub     ./examples/quickstart/sub

# ── monitor image ──────────────────────────────────────────────────────────────
FROM alpine:3.20 AS monitor
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/monitor /usr/local/bin/monitor
EXPOSE 8080
ENV MONITOR_ADDR=:8080
ENTRYPOINT ["monitor"]

# ── publisher image ────────────────────────────────────────────────────────────
FROM alpine:3.20 AS pub
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/pub /usr/local/bin/pub
ENTRYPOINT ["pub"]

# ── subscriber image ───────────────────────────────────────────────────────────
FROM alpine:3.20 AS sub
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/sub /usr/local/bin/sub
ENTRYPOINT ["sub"]
