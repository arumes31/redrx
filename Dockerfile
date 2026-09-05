# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# ---- build ----
FROM golang:1.27.1-alpine3.24@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

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
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

# ca-certificates is needed to fetch the phishing blocklists over HTTPS;
# tzdata so timestamps render correctly outside UTC.
RUN apk add --no-cache \
        ca-certificates=20260611-r0 \
        libcrypto3=3.5.8-r0 \
        libidn2=2.3.8-r0 \
        libssl3=3.5.8-r0 \
        libunistring=1.4.2-r0 \
        pcre2=10.47-r1 \
        tzdata=2026c-r0 \
        wget=1.25.0-r3 \
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
