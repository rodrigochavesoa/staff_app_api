# Build stage
FROM golang:1.25.12-alpine AS builder

WORKDIR /build

# Install git and certificates
RUN apk add --no-cache git ca-certificates

# Copy dependency manifests
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary (disable CGO, optimize for size, static build)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o staff-api ./cmd/api

# Final stage
FROM alpine:3.21

WORKDIR /app

# ca-certificates/tzdata for runtime; wget for docker-compose healthcheck
RUN apk add --no-cache ca-certificates tzdata wget

# Create application user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Create directories for SQLite db (volume mount point), static data assets, and Garmin uploads
RUN mkdir -p /app/data/db /app/uploads_atividades && \
    chown -R appuser:appgroup /app

# Copy the compiled binary and library files from the builder and set ownership
COPY --from=builder --chown=appuser:appgroup /build/staff-api /app/staff-api
COPY --from=builder --chown=appuser:appgroup /build/data/csv /app/data/csv
COPY --from=builder --chown=appuser:appgroup /build/data/json /app/data/json

# Expose port (default 5000)
EXPOSE 5000

# Define env defaults inside container (can be overridden)
ENV PORT=5000
ENV ENV=production
ENV DB_PATH=/app/data/db/fichas_treino.db
ENV GARMIN_UPLOAD_DIR=/app/uploads_atividades

# Run as non-root user
USER appuser

# Run the API server
ENTRYPOINT ["/app/staff-api"]
