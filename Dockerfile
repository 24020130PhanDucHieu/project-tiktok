# syntax=docker/dockerfile:1

# Stage 1: Build Go backend
FROM golang:alpine AS go-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# Stage 2: Install Zalo Bridge dependencies
FROM node:22-alpine AS bridge-builder
WORKDIR /app/zalo-bridge
COPY zalo-bridge/package*.json ./
RUN npm ci --omit=dev
COPY zalo-bridge/ ./

# Stage 3: Unified production runtime (Runs Go-Fiber API + Zalo Bridge in 1 service)
FROM node:22-alpine
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app

COPY --from=go-builder /out/api ./api
COPY --from=go-builder /out/seed ./seed
COPY --from=bridge-builder /app/zalo-bridge ./zalo-bridge

ENV PORT=5000 APP_ENV=production ZALO_BRIDGE_URL=http://127.0.0.1:5100 ZALO_BRIDGE_SECRET=development-bridge-secret
EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:${PORT}/health || exit 1

# When Go starts, it automatically spawns node zalo-bridge/src/server.js in the background
ENTRYPOINT ["/app/api"]
