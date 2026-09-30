# syntax=docker/dockerfile:1
# Headless ADM: download engine + web UI on port 8080.
#   docker build -t adm .
#   docker run -p 8080:8080 -e ADM_PASSWORD=change-me -v adm-data:/data -v ~/Downloads:/downloads adm

FROM node:24-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/frontend/dist ./frontend/dist
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags server -trimpath -ldflags "-s -w" -o /out/adm-server . \
 && mkdir -p /out/data /out/downloads

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/adm-server /adm-server
# Pre-create the volume dirs owned by the nonroot user (uid 65532) so named
# volumes are writable out of the box.
COPY --from=server --chown=65532:65532 /out/data /data
COPY --from=server --chown=65532:65532 /out/downloads /downloads
ENV ADM_LISTEN=:8080 \
    ADM_DATA_DIR=/data \
    ADM_DOWNLOAD_ROOT=/downloads
VOLUME ["/data", "/downloads"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/adm-server", "healthcheck"]
ENTRYPOINT ["/adm-server"]
