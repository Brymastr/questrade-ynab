.PHONY: help dev backend frontend install build build-backend build-frontend build-lambda deploy clean

# air (Go live reload) — referenced by absolute path so it works even when
# $(go env GOPATH)/bin isn't on PATH.
AIR := $(shell go env GOPATH)/bin/air

# Default target: show available commands
help:
	@echo "Targets:"
	@echo "  make dev       - run backend (:8080) and frontend (:5173) together"
	@echo "  make backend   - run the Go API server + scheduler (live reload via air)"
	@echo "  make frontend  - run the Vite dev server"
	@echo "  make install   - install frontend npm dependencies"
	@echo "  make build       - build backend binary and frontend bundle"
	@echo "  make build-lambda- cross-compile the arm64 Lambda bootstrap (infra/assets)"
	@echo "  make deploy      - build artifacts + cdk deploy to AWS"
	@echo "  make clean       - remove build artifacts"

# --- Development -----------------------------------------------------------

# Run both backend and frontend; Ctrl-C stops both.
dev:
	@echo "Starting backend (:8080) and frontend (:5173)..."
	@trap 'kill 0' INT TERM EXIT; \
		$(MAKE) backend & \
		$(MAKE) frontend & \
		wait

# Live-reloads on .go/.env changes via air (see .air.toml). Installs air on
# first run if it isn't present.
backend:
	@test -x "$(AIR)" || { echo "Installing air (live reload)..."; go install github.com/air-verse/air@latest; }
	@"$(AIR)"

frontend:
	cd web && npm run dev -- --host

# --- Setup -----------------------------------------------------------------

install:
	cd web && npm install

# --- Build -----------------------------------------------------------------

build: build-backend build-frontend

build-backend:
	go build -o bin/questrade-ynab .

build-frontend:
	cd web && npm run build

# --- Deploy (AWS Lambda + CloudFront via CDK) ------------------------------

# Cross-compile the Go binary as the Lambda custom-runtime entrypoint (arm64).
# modernc/sqlite is pure Go, so CGO stays disabled.
build-lambda:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o infra/assets/bootstrap .

# Build both artifacts then deploy the CDK stack. Requires DOMAIN_NAME,
# HOSTED_ZONE_NAME and the OAuth secrets exported (see infra/README.md).
deploy: build-lambda build-frontend
	cd infra && npx cdk deploy

# --- Cleanup ---------------------------------------------------------------

clean:
	rm -rf bin tmp web/dist infra/assets infra/cdk.out
