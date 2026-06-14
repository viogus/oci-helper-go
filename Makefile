.PHONY: dev build clean

dev:
	cd frontend && npm run dev

build:
	cd frontend && npm run build
	rm -rf internal/handler/dist
	cp -r frontend/dist internal/handler/dist
	CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server

clean:
	rm -rf oci-helper frontend/dist internal/handler/dist frontend/node_modules
