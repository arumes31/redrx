# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the application
# -ldflags="-w -s" reduces binary size by stripping debug info
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Final Stage
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/server .

# Copy migration files if needed at runtime
COPY --from=builder /app/migration ./migration

# Copy static assets and templates if they are not embedded
# COPY --from=builder /app/web ./web

EXPOSE 8080

CMD ["./server"]
