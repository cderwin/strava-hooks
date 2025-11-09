build:
    docker build . -t strava-hooks

start:
    docker run --rm --env-file=.env -p 8080:8080 strava-hooks

# Docker Compose commands for local end-to-end testing
compose-up:
    docker compose up --build -d

compose-down:
    docker compose down

compose-logs:
    docker compose logs -f

compose-restart:
    docker compose restart app

# Full stack management
dev-up: compose-up
    @echo "Stack is running at http://localhost:8080"
    @echo "Redis is available at localhost:6379"
    @echo "View logs with: just compose-logs"

dev-down: compose-down
    @echo "Stack stopped"

# Run tests
test:
    go test ./...
