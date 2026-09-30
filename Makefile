# Common tasks. Desktop builds need the GTK/WebKit headers on Linux
# (or use `make docker-desktop`, which needs only Docker).
.PHONY: dev build install uninstall server test docker docker-desktop

dev:            ## desktop app with hot reload
	wails dev -tags webkit2_41

build:          ## desktop app -> build/bin/adm
	wails build -tags webkit2_41 -trimpath

install: build  ## add ADM to your app launcher (~/.local)
	scripts/install-linux.sh

uninstall:
	scripts/install-linux.sh --uninstall

server:         ## headless server -> build/bin/adm-server (pure Go, no GTK)
	cd frontend && npm run build
	CGO_ENABLED=0 go build -tags server -trimpath -ldflags "-s -w" -o build/bin/adm-server .

test:
	go test -race ./internal/...

docker:         ## headless server image
	docker build -t adm .

docker-desktop: ## build Linux + Windows desktop binaries in Docker -> dist/
	docker build -f build/docker/desktop.Dockerfile --output dist .
