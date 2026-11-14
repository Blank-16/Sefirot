ROOT := $(CURDIR)
BACKEND := $(ROOT)/backend

.PHONY: run build test fmt verify build-all frontend desktop-sidecar desktop-build

run:
	cd backend && go run .

build:
	cd backend && go build -trimpath -ldflags='-s -w' -o ../sefirot .

test:
	cd backend && go test ./...

fmt:
	find backend -name '*.go' -print0 | xargs -0 gofmt -w

verify:
	test -z "$$(find backend -name '*.go' -print0 | xargs -0 gofmt -l)"
	for file in backend/web/app.js backend/web/js/*.js; do node --check "$$file"; done
	cd backend && go test ./...

build-all:
	mkdir -p dist
	cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-windows-amd64.exe .
	cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-windows-arm64.exe .
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-linux-amd64 .
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-linux-arm64 .
	cd backend && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-macos-amd64 .
	cd backend && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ../dist/sefirot-macos-arm64 .

frontend:
	npm install
	npm run frontend:build

desktop-sidecar:
	./scripts/build-desktop-sidecar.sh

desktop-build:
	./scripts/build-desktop.sh
