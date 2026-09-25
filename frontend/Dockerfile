# --- build ------------------------------------------------------------------
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# --- runtime ----------------------------------------------------------------
# nginx serves the SPA and proxies /api to the Go service, so the browser only
# talks to one origin and the Anthropic key never leaves the backend.
FROM nginx:1.27-alpine
# The official image renders /etc/nginx/templates/*.template with envsubst on start.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
ENV PORT=80 API_UPSTREAM=api:8080
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" || exit 1
