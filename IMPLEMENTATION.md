# RealmRunner Implementation Specification

## Project Overview
RealmRunner is a web-based Minecraft Java Edition server manager packaged as a Docker container. It allows users to create, manage, and control multiple Minecraft servers through a simple web interface.

## Core Features

### User Stories
1. User opens webpage and sees list of all servers (running and stopped)
2. User can create new server by selecting from available Minecraft versions
3. User can start/stop servers
4. User can wipeout server data (delete world and reset)
5. User can view real-time server logs
6. User can send console commands to running servers
7. System enforces maximum number of running servers

### Feature Requirements

#### Server Management
- **Create Server**: Download official Minecraft server JAR, extract, auto-accept EULA, configure
- **Start Server**: Launch Minecraft server process with configured memory
- **Stop Server**: Gracefully stop server process (30s timeout before force kill)
- **Wipeout**: Delete all server data (world, logs, configs) and reset to fresh state
- **List Servers**: Display all servers with status (stopped, starting, running, stopping)

#### Version Management
- Java Edition only (official Mojang servers)
- Fetch available versions from Mojang version manifest API
- Display versions in dropdown for user selection
- Cache downloaded server JARs to avoid re-downloading

#### Port Management
- User-specified ports within configured range
- Port conflict detection and validation
- Display assigned port in UI for server access
- No RCON port exposure

#### Console & Logs
- Real-time log streaming via WebSocket
- Console command execution (e.g., `/say`, `/op`, `/gamemode`)
- Log tailing from server log file

#### Resource Monitoring (Optional - if easy)
- CPU usage per server
- Memory usage per server
- Player count
- Uptime

## Technical Architecture

### Tech Stack
- **Backend**: Go with Gin framework
- **Frontend**: Vue 3 (Composition API) + Vite
- **Database**: SQLite for server metadata
- **Storage**: Filesystem for server data
- **Auth**: Password-based (bcrypt hash)
- **Communication**: REST API + WebSockets

### Container Architecture
- Single Docker container
- Minecraft servers run as child processes (not separate containers)
- OpenJDK baked into Docker image
- Single volume mount for all server data

### Directory Structure
```
/data/                          # Volume mount point
  └── servers/
      ├── <server-uuid-1>/
      │   ├── server.jar
      │   ├── server.properties
      │   ├── eula.txt
      │   ├── world/
      │   └── logs/
      └── <server-uuid-2>/
          └── ...

/app/
  ├── backend/                  # Go application
  │   ├── main.go
  │   ├── auth/
  │   ├── api/
  │   ├── minecraft/
  │   ├── server/
  │   └── websocket/
  ├── frontend/                 # Vue source
  │   ├── src/
  │   └── package.json
  └── dist/                     # Built frontend (served by Go)
```

### Environment Variables
```bash
REALMRUNNER_PASSWORD_HASH=<bcrypt hash>    # Auth password (bcrypt hashed)
REALMRUNNER_JWT_SECRET=<secret>            # JWT signing secret
REALMRUNNER_MAX_RUNNING=3                  # Max concurrent running servers
REALMRUNNER_PORT_RANGE=25565-25600         # Allowed port range for servers
REALMRUNNER_MEMORY_MB=2048                 # Memory allocation per server
REALMRUNNER_DATA_DIR=/data                 # Data directory path
REALMRUNNER_BASE_URL=realmrunner.ziniewicz.eu  # Display domain for connections
```

### Database Schema (SQLite)
```sql
CREATE TABLE servers (
    id TEXT PRIMARY KEY,              -- UUID
    name TEXT NOT NULL,               -- User-provided or auto-generated name
    version TEXT NOT NULL,            -- Minecraft version (e.g., "1.20.1")
    port INTEGER NOT NULL UNIQUE,     -- Server port
    status TEXT NOT NULL,             -- stopped, starting, running, stopping
    created_at TIMESTAMP NOT NULL,    -- Server creation time
    last_started_at TIMESTAMP         -- Last time server was started
);
```

### API Specification

#### REST Endpoints
```
POST   /api/auth/login              # Login with password, returns JWT
GET    /api/servers                 # List all servers
POST   /api/servers                 # Create new server
GET    /api/servers/:id             # Get server details
DELETE /api/servers/:id/wipeout     # Wipeout server data
POST   /api/servers/:id/start       # Start server
POST   /api/servers/:id/stop        # Stop server
POST   /api/servers/:id/command     # Send console command
GET    /api/versions                # Get available Minecraft versions
```

#### WebSocket Endpoint
```
WS     /api/ws/:id                  # Real-time logs and status updates
```

#### Request/Response Examples

**POST /api/servers** (Create Server)
```json
{
  "name": "Survival World",
  "version": "1.20.1",
  "port": 25565
}
```

**POST /api/servers/:id/command** (Send Command)
```json
{
  "command": "say Hello, players!"
}
```

**WebSocket Messages** (Server -> Client)
```json
{
  "type": "log",
  "timestamp": "2025-10-10T12:34:56Z",
  "message": "[Server thread/INFO]: Done (5.234s)!"
}
```

```json
{
  "type": "status",
  "status": "running"
}
```

### Authentication
- Single password authentication (no user accounts, no RBAC)
- Password stored as bcrypt hash in environment variable
- JWT tokens for session management
- All API endpoints (except /auth/login) require authentication
- Frontend stores JWT in localStorage

### Server Configuration
- **Naming**: User-provided names during creation
- **Memory**: Fixed allocation via env var (e.g., 2048 MB)
- **EULA**: Auto-accepted during server creation
- **server.properties**: Default configuration, no UI customization (future feature)
- **Difficulty**: Default (Easy)
- **Game Mode**: Default (Survival)
- **Max Players**: Default (20)
- **Graceful Shutdown**: 30 second timeout before force kill

### Minecraft Version Fetching
- Use Mojang version manifest API: https://launchermeta.mojang.com/mc/game/version_manifest.json
- Filter for release versions (exclude snapshots by default)
- Cache manifest locally with TTL (e.g., 1 hour)
- Download server.jar from URL in version manifest
- Cache downloaded server JARs by version to avoid re-downloading (optional optimization)

### Process Management
- Track server processes (PID, status)
- Handle stdout/stderr for log streaming
- Graceful shutdown with timeout
- Auto-restart on crash (optional feature)
- Prevent zombie processes

## Implementation Phases

### Phase 1: Core Backend
- [ ] Go project structure setup
- [ ] SQLite database initialization
- [ ] Auth middleware with password verification
- [ ] Server CRUD API handlers
- [ ] Minecraft version manifest fetcher
- [ ] Configuration loading from env vars

### Phase 2: Server Lifecycle
- [ ] Server creation flow (download, extract, setup)
- [ ] Process management (start, stop, status tracking)
- [ ] EULA auto-acceptance
- [ ] Port validation and conflict detection
- [ ] Wipeout functionality

### Phase 3: WebSocket & Console
- [ ] WebSocket connection management
- [ ] Log file tailing and streaming
- [ ] Console command execution via stdin
- [ ] Real-time status updates
- [ ] Connection pooling and cleanup

### Phase 4: Frontend
- [ ] Vue 3 + Vite project setup
- [ ] Login page with password authentication
- [ ] Dashboard with server list
- [ ] Server card component (status, controls)
- [ ] Create server modal with version dropdown
- [ ] Console modal (logs display + command input)
- [ ] WebSocket client integration
- [ ] Error handling and user feedback

### Phase 5: Docker & Deployment
- [ ] Multi-stage Dockerfile (build frontend, build backend)
- [ ] OpenJDK installation in image
- [ ] Volume configuration
- [ ] Entrypoint script
- [ ] docker-compose.yml example
- [ ] Health check endpoint

## Design Decisions & Constraints

### Decisions Made
1. **Single password auth**: Simple but sufficient for trusted environments
2. **SQLite**: Lightweight, no external dependencies, sufficient for use case
3. **Processes not containers**: Simpler management, shared resources acceptable
4. **Auto EULA**: Streamlines setup, assumes user compliance
5. **Fixed memory**: Simplifies resource management
6. **No backups**: Keeps scope minimal, can add later

### Known Limitations
1. **Port conflicts**: Basic validation, but no automatic reassignment
2. **No user accounts**: Single password for all users
3. **No rate limiting**: Trust-based system
4. **No server.properties UI**: Must edit files manually
5. **No backup/restore**: Permanent data loss on wipeout
6. **No resource limits**: Servers can consume available system resources

### Future Enhancements (Not in Scope)
- [ ] User accounts and permissions
- [ ] Automatic backups and snapshots
- [ ] Server.properties editor in UI
- [ ] Plugin/mod support (Paper, Spigot, Fabric, Forge)
- [ ] Scheduled restarts
- [ ] Resource usage alerts
- [ ] Server templates
- [ ] Whitelist/ops management UI
- [ ] File browser for server files

## Security Considerations

### Authentication
- HTTPS recommended for production (reverse proxy)
- JWT token expiration and refresh
- Rate limiting on login endpoint
- Password complexity requirements

### Server Management
- Input validation for commands (prevent shell injection)
- Port range enforcement
- File path validation (prevent directory traversal)
- Resource limits per server process

### Network
- Firewall rules for Minecraft ports
- Expose only necessary ports from Docker
- Consider VPN for admin access

## Testing Strategy

### Unit Tests
- Minecraft version fetcher
- Port validation logic
- Process management
- Auth middleware

### Integration Tests
- Server creation flow end-to-end
- Start/stop/wipeout operations
- WebSocket communication
- Log streaming

### Manual Testing
- Create and start multiple servers
- Verify max running limit enforcement
- Test console commands
- Verify logs display correctly
- Test port conflict handling

## Deployment

### Docker Image
```dockerfile
FROM node:20-alpine AS frontend-builder
# Build Vue frontend

FROM golang:1.21 AS backend-builder
# Build Go backend

FROM eclipse-temurin:21-jre
# Install ca-certificates
# Copy built backend binary
# Copy built frontend static files
```

### Deployment

Deployed via Komodo with SSL terminated by Traefik. See `compose.yaml` for the full configuration. The `traefik_proxy` external Docker network connects the container to Traefik.

## Decisions Finalized
1. **Server naming**: User-provided names ✓
2. **Java runtime**: OpenJDK baked into Docker image ✓
3. **Server jar caching**: Yes, cache by version (optional optimization) ✓
4. **Graceful shutdown timeout**: 30 seconds ✓

## References
- Mojang Version Manifest: https://launchermeta.mojang.com/mc/game/version_manifest.json
- Minecraft Server Properties: https://minecraft.fandom.com/wiki/Server.properties
- Docker Best Practices: https://docs.docker.com/develop/dev-best-practices/
