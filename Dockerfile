# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27-alpine AS build

WORKDIR /src

# Download modules first so dependency layers cache across source edits.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO is off because the SQLite driver is pure Go, which keeps the runtime image
# free of libc and lets the binary run on any base.
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/redrx ./cmd/redrx

# ---- runtime ----
FROM alpine:3.24

# ca-certificates is needed to fetch the phishing blocklists over HTTPS;
# tzdata so timestamps render correctly outside UTC.
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S redrx && adduser -S -G redrx redrx \
    && mkdir -p /app/db && chown -R redrx:redrx /app

WORKDIR /app
COPY --from=build /out/redrx /usr/local/bin/redrx

# HTML templates and static assets are compiled into the binary, so nothing
# else needs to be copied.

USER redrx

ENV BASE_DOMAIN=short.example.com \
    EXPIRY_HOURS=24 \
    SHORT_CODE_LENGTH=6 \
    DEFAULT_QR_COLOR=black \
    DEFAULT_QR_BACKGROUND=white \
    LISTEN_ADDR=:5000

EXPOSE 5000

# Probe whatever LISTEN_ADDR binds. A wildcard or empty host is reached over
# loopback; an explicit host is probed directly.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD host="${LISTEN_ADDR%:*}"; port="${LISTEN_ADDR##*:}"; \
        case "$host" in ''|'0.0.0.0'|'[::]'|'::') host='127.0.0.1' ;; esac; \
        wget -qO- "http://$host:$port/health" >/dev/null || exit 1

ENTRYPOINT ["redrx"]
