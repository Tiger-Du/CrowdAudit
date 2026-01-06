# ---- build stage ----
FROM golang:1.24-alpine AS build

WORKDIR /app

# Enable static binary
ENV CGO_ENABLED=0

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build only the inference API binary
RUN go build -trimpath -ldflags="-s -w" \
    -o inference-api ./cmd/inference-api

# ---- runtime stage ----
FROM gcr.io/distroless/base-debian12

WORKDIR /

# Copy binary
COPY --from=build /app/inference-api /inference-api

# Use non-root user
USER nonroot:nonroot

# Expose API + metrics ports
EXPOSE 8080
EXPOSE 2112

ENTRYPOINT ["/inference-api"]
