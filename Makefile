# Tract — convenience targets. Plain make, macOS/Linux portable.

.PHONY: build test vet lint run frontend-dev clean

build: ## build frontend + single binary
	./scripts/build.sh

test: ## run Go tests
	go test ./...

vet: ## static checks
	go vet ./...

lint: ## anti-slop CSS lint (deterministic, no deps)
	node scripts/lint-css.mjs

run: build ## build then run the server (:8080)
	./bin/tract

frontend-dev: ## vite dev server with /api proxy to :8080
	cd frontend && npm run dev

clean:
	rm -rf bin frontend/dist cmd/tract/dist/assets
