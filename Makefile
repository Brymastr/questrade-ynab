.PHONY: help dev backend frontend install build build-backend build-frontend clean

# Default target: show available commands
help:
	@echo "Targets:"
	@echo "  make dev       - run backend (:8080) and frontend (:5173) together"
	@echo "  make backend   - run the Go API server + scheduler"
	@echo "  make frontend  - run the Vite dev server"
	@echo "  make install   - install frontend npm dependencies"
	@echo "  make build     - build backend binary and frontend bundle"
	@echo "  make clean     - remove build artifacts"

# --- Development -----------------------------------------------------------

# Run both backend and frontend; Ctrl-C stops both.
dev:
	@echo "Starting backend (:8080) and frontend (:5173)..."
	@trap 'kill 0' INT TERM EXIT; \
		$(MAKE) backend & \
		$(MAKE) frontend & \
		wait

backend:
	go run . serve

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

# --- Cleanup ---------------------------------------------------------------

clean:
	rm -rf bin web/dist
