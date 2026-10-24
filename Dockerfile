# Stage 1: Build Next.js static export
FROM node:22-alpine AS web-builder
WORKDIR /src/web
COPY web/package.json ./
RUN npm install
COPY web/ ./
RUN npm run build

# Stage 2: Build Go single binary with embedded frontend
FROM golang:1.26-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN cp -r /src/web/out/* /src/internal/webui/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cortex ./cmd/cortex

# Stage 3: Minimal runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata bash curl python3
WORKDIR /app
COPY --from=go-builder /out/cortex /app/cortex
COPY skills/ /app/skills/
VOLUME ["/data"]
ENV PORT=8080 \
    CORTEX_DB_PATH=/data/cortex.db \
    CORTEX_SKILLS_DIR=/app/skills \
    CORTEX_LOCALE=en
EXPOSE 8080
ENTRYPOINT ["/app/cortex"]
