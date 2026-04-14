FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /pinger .

# Final minimal image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /pinger /pinger

ENTRYPOINT ["/pinger"]
