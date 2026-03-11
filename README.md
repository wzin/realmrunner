# RealmRunner

A web-based Minecraft Java Edition server manager packaged as a Docker container. Easily create, manage, and control multiple Minecraft servers through a simple web interface.

## Features

- **Multiple Servers**: Create and manage multiple Minecraft servers
- **Version Selection**: Choose from official Minecraft Java Edition releases
- **Server Controls**: Start, stop, and wipeout servers with one click
- **Real-time Logs**: View server logs as they happen
- **Console Access**: Send commands directly to running servers
- **Password Protected**: Secure access with password authentication
- **Docker Ready**: Single container with all dependencies included

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- Ports available for Minecraft servers (default: 25565-25600)

### Running with Docker Compose

1. Clone the repository:

```bash
git clone git@github.com:wzin/realmrunner.git
cd realmrunner
```

2. Generate a password hash:

```bash
python3 gen_password.py yourpassword
```

3. Create a `.env` file:

```bash
REALMRUNNER_PASSWORD_HASH=$$2b$$12$$YourHashHere
REALMRUNNER_JWT_SECRET=your-random-secret
```

4. Build and start:

```bash
docker compose build
docker compose up -d
```

5. Access the web UI at `http://localhost:8080` (if port 8080 is uncommented in compose.yaml)

### Production Deployment with Traefik

RealmRunner is designed to run behind Traefik for SSL termination. The `compose.yaml` includes Traefik labels that route `realmrunner.ziniewicz.eu` to the container. In production, port 8080 is not exposed directly -- Traefik routes to it via the Docker network.

For Komodo deployment, set `REALMRUNNER_PASSWORD_HASH` and `REALMRUNNER_JWT_SECRET` as environment variables in the Komodo stack configuration. Use the **unescaped** bcrypt hash (single `$` signs) in Komodo's UI.

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `REALMRUNNER_PASSWORD_HASH` | Bcrypt hash of your password | - | Yes |
| `REALMRUNNER_JWT_SECRET` | JWT signing secret | - | Yes |
| `REALMRUNNER_MAX_RUNNING` | Maximum number of running servers | 3 | No |
| `REALMRUNNER_PORT_RANGE` | Port range for Minecraft servers | 25565-25600 | No |
| `REALMRUNNER_MEMORY_MB` | Memory allocation per server (MB) | 2048 | No |
| `REALMRUNNER_DATA_DIR` | Data directory path | /data | No |
| `REALMRUNNER_BASE_URL` | Domain for server connection display | localhost | No |

### Generating a Password Hash

```bash
# Using the included Python script (outputs $$-escaped hash for compose/env files)
python3 gen_password.py yourpassword

# Using Docker with Python
docker run --rm python:3.11-slim sh -c \
  "pip install -q bcrypt && python -c 'import bcrypt; print(bcrypt.hashpw(b\"yourpassword\", bcrypt.gensalt()).decode())'"
```

**Note:** In `.env` files and `compose.yaml`, `$` signs must be escaped as `$$`. In Komodo or other UIs that set env vars directly, use the unescaped hash.

## Usage

### Creating a Server

1. Click the **"Create Server"** button
2. Enter a name for your server
3. Select a Minecraft version from the dropdown
4. Specify a port (within configured range)
5. Click **"Create"**

### Starting a Server

Click the **"Start"** button on a server card. Connect using the displayed address (e.g., `realmrunner.ziniewicz.eu:25565`).

### Stopping a Server

Click the **"Stop"** button. The server will gracefully shut down (30 second timeout).

### Viewing Logs & Console

1. Click on a server card to open the console
2. View real-time logs as they stream
3. Enter commands in the input field (e.g., `/say Hello!`)
4. Press Enter to send commands

### Wiping Server Data

Click the **"Wipeout"** button to permanently delete all server data. This cannot be undone.

## Data Persistence

All server data is stored in the `/data` volume:

```
/data/
  └── servers/
      ├── <server-uuid-1>/
      │   ├── server.jar
      │   ├── server.properties
      │   ├── world/
      │   └── logs/
      └── <server-uuid-2>/
          └── ...
```

Mount a host directory to `/data` to persist servers across container restarts.

## Resource Requirements

### Per Server
- **RAM**: Configured via `REALMRUNNER_MEMORY_MB` (default: 2GB)
- **CPU**: ~1-2 cores per server (varies by player count)
- **Disk**: ~500MB minimum per server (grows with world size)

### Recommended Host Specs
- **3 servers**: 8GB RAM, 4 CPU cores, 20GB disk
- **5 servers**: 12GB RAM, 8 CPU cores, 50GB disk

## Security

### Authentication
- Password-based authentication with bcrypt hashing
- All users share the same password (no user accounts)
- SSL terminated by Traefik in production

### Network Security
- Only Minecraft ports (25565-25600) are exposed to the host
- Web UI is accessed via Traefik reverse proxy (not exposed directly in production)
- Use firewall rules to restrict Minecraft port access

### Best Practices
- Use a strong password (16+ characters)
- Regularly backup the `/data` volume
- Keep Docker image updated

## Building from Source

```bash
git clone git@github.com:wzin/realmrunner.git
cd realmrunner
docker compose build
docker compose up -d
```

The Dockerfile uses a multi-stage build: Node.js for the Vue frontend, Go for the backend, and eclipse-temurin:21-jre for the runtime.

## Development

### Backend

```bash
cd backend
go mod download
export REALMRUNNER_PASSWORD_HASH="your-hash"
export REALMRUNNER_DATA_DIR="./data"
go run .
```

### Frontend

```bash
cd frontend
npm install
npm run dev  # Dev server at http://localhost:5173, proxies API to :8080
```

See `IMPLEMENTATION.md` for detailed implementation specifications.
See `CLAUDE.md` for development context and architecture decisions.

## License

MIT License - see LICENSE file for details

## Support

- Issues: https://github.com/wzin/realmrunner/issues
- Pull Requests: https://github.com/wzin/realmrunner/pulls
