# Single image: the Go API also serves the built frontend (STATIC_DIR).
# Used by free single-service hosts (Render, Koyeb, Cloud Run). The
# docker-compose.yml stack keeps separate api / web images.

# --- frontend -----------------------------------------------------------------
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# --- backend ------------------------------------------------------------------
FROM golang:1.24-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime ------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=api /out/api /app/api
COPY --from=web /web/dist /app/web
# expected_results.csv is never copied: only the files the app reads.
COPY backend/data/readings.csv backend/data/events.csv backend/data/meters.csv /app/data/
ENV PORT=8080 DATA_DIR=/app/data STATIC_DIR=/app/web
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=4s --retries=5 CMD ["/app/api", "healthcheck"]
ENTRYPOINT ["/app/api"]
