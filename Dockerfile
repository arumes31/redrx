# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# ---- build ----
FROM golang:1.27.3-alpine3.24@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build

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
FROM alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# ca-certificates is needed to fetch the phishing blocklists over HTTPS;
# tzdata so timestamps render correctly outside UTC.
# Alpine mirrors replace superseded package revisions. Resolve security updates
# from the pinned release's repositories; the image scan verifies the result.
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S -g 10001 redrx \
    && adduser -S -D -H -u 10001 -G redrx redrx \
    && install -d -o 10001 -g 10001 -m 0750 /app/db /app/data

WORKDIR /app
COPY --from=build /out/redrx /usr/local/bin/redrx

# HTML templates and static assets are compiled into the binary, so nothing
# else needs to be copied.

USER 10001:10001

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
