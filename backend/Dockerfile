# --- build ------------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- runtime ----------------------------------------------------------------
# Distroless: no shell, no package manager, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /app/api
# Only readings/events/meters are shipped. expected_results.csv is excluded in
# .dockerignore: it is ground truth reserved to the evaluator.
COPY data/readings.csv data/events.csv data/meters.csv /app/data/
ENV PORT=8080 DATA_DIR=/app/data
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=4s --retries=5 CMD ["/app/api", "healthcheck"]
ENTRYPOINT ["/app/api"]
