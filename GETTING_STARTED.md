# Getting Started with RealmRunner

## Quick Start

### 1. Generate a Password Hash

First, generate a bcrypt hash for your password:

```bash
cd scripts
go run generate-password.go yourpassword
```

Copy the generated hash.

### 2. Update docker-compose.yml

Replace the `REALMRUNNER_PASSWORD_HASH` value in `docker-compose.yml` with your generated hash.

### 3. Build and Run

```bash
# Build the Docker image
docker compose build

# Start the container
docker compose up -d

# View logs
docker compose logs -f
```

### 4. Access the Application

Open your browser to http://localhost:8080

Login with the password you used to generate the hash.

## Development Setup

### Backend Development

```bash
cd backend

# Download dependencies
go mod download

# Run backend (requires PostgreSQL or SQLite)
export REALMRUNNER_PASSWORD_HASH="your-hash"
export REALMRUNNER_DATA_DIR="./data"
go run .
```

### Frontend Development

```bash
cd frontend

# Install dependencies
npm install

# Run dev server (proxies API to backend)
npm run dev
```

Frontend will be available at http://localhost:5173

### Building

```bash
# Build frontend
cd frontend
npm run build

# Build backend
cd backend
go build -o realmrunner

# Run
./realmrunner
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `REALMRUNNER_PASSWORD_HASH` | Yes | - | Bcrypt hash of password |
| `REALMRUNNER_MAX_RUNNING` | No | 3 | Max concurrent servers |
| `REALMRUNNER_PORT_RANGE` | No | 25565-25600 | Port range for servers |
| `REALMRUNNER_MEMORY_MB` | No | 2048 | Memory per server (MB) |
| `REALMRUNNER_DATA_DIR` | No | /data | Data directory |
| `REALMRUNNER_JWT_SECRET` | No | auto | JWT signing secret |

## Project Structure

```
realmrunner/
├── backend/              # Go backend
│   ├── api/             # HTTP handlers
│   ├── auth/            # Authentication
│   ├── config/          # Configuration
│   ├── minecraft/       # Version fetching & download
│   ├── server/          # Server management
│   └── websocket/       # WebSocket for logs
├── frontend/            # Vue frontend
│   └── src/
│       ├── components/  # Vue components
│       ├── views/       # Page views
│       ├── api/         # API client
│       └── router/      # Vue Router
├── scripts/             # Utility scripts
├── Dockerfile           # Multi-stage build
└── docker-compose.yml   # Deployment config
```

## Common Tasks

### Create a New Server

1. Click "Create Server"
2. Enter name, select version, specify port
3. Wait for download to complete
4. Click "Start"

### View Server Console

1. Click on a running server's "Console" button
2. View logs in real-time
3. Send commands via input field

### Stop a Server

1. Click "Stop" on the server card
2. Wait for graceful shutdown (30s max)

### Wipeout Server Data

1. Stop the server first
2. Click "Wipeout"
3. Confirm deletion (permanent!)

## Troubleshooting

### Backend won't start

- Check `REALMRUNNER_PASSWORD_HASH` is set
- Verify data directory exists and is writable
- Check logs: `docker compose logs realmrunner`

### Frontend build fails

- Delete `node_modules` and `package-lock.json`
- Run `npm install` again

### Server won't start

- Check Java is installed in container
- Verify server.jar was downloaded
- Check port isn't already in use
- View server logs in console

### WebSocket connection fails

- Ensure server is running
- Check browser console for errors
- Verify JWT token is valid

## Next Steps

- Read [IMPLEMENTATION.md](IMPLEMENTATION.md) for technical details
- Read [README.md](README.md) for user documentation
- Read [CLAUDE.md](CLAUDE.md) for development context

## Support

- GitHub Issues: https://github.com/wzin/realmrunner/issues
- Documentation: Check the docs in this repository
