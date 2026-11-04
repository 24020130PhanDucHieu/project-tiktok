# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
ARG VERSION=dev
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/api /out/seed ./
RUN mkdir -p /app/logs && chown app:app /app/logs
USER app
ENV PORT=5000 APP_ENV=production
EXPOSE 5000
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:${PORT}/health || exit 1
ENTRYPOINT ["/app/api"]
