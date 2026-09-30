# syntax=docker/dockerfile:1
# Build environment for the ADM desktop app, so nothing needs to be installed
# on the host. Produces Linux and Windows binaries:
#
#   docker build -f build/docker/desktop.Dockerfile --output dist .
#
# Ubuntu 22.04 is used on purpose: binaries built against its glibc run on
# most current distributions. macOS builds need a Mac (see the CI workflow).

FROM ubuntu:22.04 AS build
ARG GO_VERSION=1.27.1
ARG NODE_VERSION=24.9.0
ARG WAILS_VERSION=v2.16.0
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential pkg-config ca-certificates curl xz-utils git \
      libgtk-3-dev libwebkit2gtk-4.1-dev \
 && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz \
 && curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" | tar -C /usr/local --strip-components=1 -xJ
ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH
RUN go install github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY frontend/package.json frontend/package-lock.json ./frontend/
RUN cd frontend && npm ci --no-audit --no-fund
COPY . .
RUN wails build -tags webkit2_41 -platform linux/amd64 -trimpath -o adm \
 && wails build -skipfrontend -platform windows/amd64 -trimpath -o adm.exe \
 && CGO_ENABLED=0 go build -tags server -trimpath -ldflags "-s -w" -o build/bin/adm-server .

FROM scratch AS export
COPY --from=build /src/build/bin/ /
