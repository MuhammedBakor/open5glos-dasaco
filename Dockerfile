FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o 5glos-gateway ./cmd

FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary
COPY --from=builder /app/5glos-gateway .

# Copy default config
COPY --from=builder /app/config.yaml .

# Create non-root user
RUN adduser -D -s /bin/sh 5glos
USER 5glos

# Expose ports
EXPOSE 38412 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9090/healthz || exit 1

# Run the application
CMD ["./5glos-gateway", "-config", "config.yaml"]