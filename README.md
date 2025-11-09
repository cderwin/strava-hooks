# Skintrackr

A webhook server for Strava activity notifications with OAuth integration and Redis-based token management.

## Features

- OAuth 2.0 flow for Strava authentication
- Webhook subscriptions for activity updates
- Secure token storage with encryption
- GPX export functionality
- Docker and Docker Compose support for local development

## Quick Start with Docker Compose

The easiest way to run the application locally for end-to-end testing:

### 1. Set up environment variables

Copy the example environment file and fill in your Strava credentials:

```bash
cp .env.example .env
```

Edit `.env` and set:
- `STRAVA_CLIENT_ID` - Get from https://www.strava.com/settings/api
- `STRAVA_CLIENT_SECRET` - Get from https://www.strava.com/settings/api
- `APP_SECRET` - Generate with: `openssl rand -hex 32`

### 2. Start the stack

Using just:
```bash
just dev-up
```

Or using docker-compose directly:
```bash
docker compose up --build -d
```

The application will be available at http://localhost:8080

### 3. View logs

```bash
just compose-logs
```

Or:
```bash
docker compose logs -f app
```

### 4. Stop the stack

```bash
just dev-down
```

Or:
```bash
docker compose down
```

## Development

### Running Tests

```bash
just test
```

Or:
```bash
go test ./...
```

### Building Docker Image Only

```bash
just build
```

### Running Without Docker Compose

If you have Redis running separately:

```bash
just start
```

## Architecture

- **App Service**: Go web server handling OAuth flow and webhooks
- **Redis Service**: Token storage and session management
- **Volume**: Persistent Redis data storage

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_BASE_URL` | No | `http://localhost:8080` | Base URL for OAuth callbacks |
| `APP_SECRET` | Yes | - | 64-character hex string for encryption (32 bytes) |
| `STRAVA_CLIENT_ID` | Yes | - | Strava OAuth client ID |
| `STRAVA_CLIENT_SECRET` | Yes | - | Strava OAuth client secret |
| `UPSTASH_REDIS_URL` | Yes* | `redis://redis:6379` | Redis connection URL |
| `DEBUG_STRAVA_RESPONSE_BODY` | No | `false` | Enable HTTP response debugging |

\* Automatically set when using docker-compose

## API Endpoints

- `GET /` - Landing page
- `GET /oauth2/connect` - Initiate OAuth flow
- `GET /oauth2/callback` - OAuth callback handler
- `GET /subscriptions/callback` - Webhook verification
- `POST /subscriptions/callback` - Webhook event handler
- `GET /healthcheck` - Health check endpoint

