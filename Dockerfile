# Full from-source build: React app + Go API in one small image.
#   docker build -t compliancekit .
#   docker run -p 8080:8080 --env-file deploy/demo.env.example compliancekit
FROM node:20-alpine AS web
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install -g npm@11.12.1 && npm ci --no-audit --no-fund
COPY frontend/ ./
ARG VITE_DEMO_MODE=1
ENV VITE_API_BASE_URL=
RUN npm run build

FROM golang:1.25-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/compliancekit-api ./cmd/server \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=api /out/compliancekit-api /app/compliancekit-api
COPY --from=web /src/dist /app/web
COPY --from=api --chown=65532:65532 /out/data /data
ENV STATIC_DIR=/app/web \
    DATABASE_URL=/data/ck.db \
    STORAGE_LOCAL_DIR=/data/blobs \
    AUTO_MIGRATE=true \
    PORT=8080
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/compliancekit-api"]
