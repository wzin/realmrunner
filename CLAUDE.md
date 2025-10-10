# RealmRunner - Development Context

This document provides context for AI assistants (like Claude) working on the RealmRunner codebase across multiple sessions.

## Project Identity

**Name**: RealmRunner
**Purpose**: Web-based Minecraft Java Edition server manager
**Repository**: git@github.com:wzin/realmrunner.git
**Target Deployment**: Docker container with single volume mount

## Architecture Overview

### Tech Stack
- **Backend**: Go 1.21+ with Gin framework
- **Frontend**: Vue 3 (Composition API) with Vite
- **Database**: SQLite for metadata
- **Storage**: Filesystem for server data
- **Auth**: Single password (bcrypt) with JWT tokens
- **Real-time**: WebSockets for logs and status

### Key Design Decisions

1. **Single Container, Multiple Processes**
   - Minecraft servers run as child processes, not separate containers
   - Simpler resource management and deployment
   - Trade-off: Less isolation between servers

2. **Single Password Authentication**
   - No user accounts or RBAC
   - Suitable for trusted environments or small teams
   - Frontend stores JWT in localStorage

3. **Fixed Memory Allocation**
   - All servers get same memory allocation (env configured)
   - Simplifies resource management
   - Future: Could add per-server configuration

4. **Auto EULA Acceptance**
   - Automatically accepts Minecraft EULA during server creation
   - Streamlines setup process
   - Assumes user is aware of and complies with EULA

5. **SQLite for Metadata**
   - Server records stored in SQLite (id, name, version, port, status)
   - Actual server data (worlds, logs) on filesystem
   - Lightweight, no external dependencies

### Directory Structure

```
/realmrunner/
├── backend/
│   ├── main.go                 # Entry point, Gin router
│   ├── go.mod
│   ├── auth/
│   │   └── middleware.go       # Password verification, JWT
│   ├── api/
│   │   ├── handlers.go         # HTTP handlers
│   │   └── routes.go           # Route definitions
│   ├── minecraft/
│   │   ├── version.go          # Mojang API integration
│   │   └── downloader.go       # JAR download & cache
│   ├── server/
│   │   ├── manager.go          # Server lifecycle
│   │   ├── process.go          # Process management
│   │   └── db.go               # SQLite operations
│   └── websocket/
│       ├── hub.go              # Connection management
│       └── client.go           # Log streaming
├── frontend/
│   ├── src/
│   │   ├── main.js             # Vue app entry
│   │   ├── App.vue             # Root component
│   │   ├── router/
│   │   │   └── index.js        # Vue Router
│   │   ├── views/
│   │   │   ├── Login.vue       # Login page
│   │   │   └── Dashboard.vue   # Main dashboard
│   │   ├── components/
│   │   │   ├── ServerCard.vue  # Server list item
│   │   │   ├── CreateModal.vue # Create server form
│   │   │   └── Console.vue     # Log viewer + command
│   │   └── api/
│   │       └── client.js       # API client with JWT
│   ├── package.json
│   └── vite.config.js
├── Dockerfile                   # Multi-stage build
├── docker-compose.yml           # Example deployment
├── IMPLEMENTATION.md            # Detailed implementation spec
├── README.md                    # User documentation
└── CLAUDE.md                    # This file
```

### Data Flow

**Server Creation**:
1. User submits form (name, version, port) via frontend
2. POST /api/servers → backend validates and creates DB record
3. Backend downloads server.jar from Mojang (or uses cache)
4. Backend creates directory structure in /data/servers/{uuid}/
5. Backend writes server.properties, accepts EULA
6. Returns server details to frontend

**Server Start**:
1. POST /api/servers/:id/start
2. Check max running limit
3. Fork process: `java -Xmx{memory}M -jar server.jar nogui`
4. Capture PID, track status
5. Stream logs via WebSocket
6. Update status to "running"

**Log Streaming**:
1. WebSocket connection to /api/ws/:id
2. Backend tails logs/latest.log
3. New lines sent as JSON messages to client
4. Client renders in console view

**Console Command**:
1. POST /api/servers/:id/command with {command: "..."}
2. Backend writes to process stdin
3. Response appears in log stream

## API Endpoints

### Authentication
- `POST /api/auth/login` - Returns JWT token
  - Body: `{password: string}`
  - Response: `{token: string}`

### Servers
- `GET /api/servers` - List all servers
- `POST /api/servers` - Create server
  - Body: `{name: string, version: string, port: number}`
- `GET /api/servers/:id` - Get server details
- `POST /api/servers/:id/start` - Start server
- `POST /api/servers/:id/stop` - Stop server (30s graceful)
- `DELETE /api/servers/:id/wipeout` - Delete all data
- `POST /api/servers/:id/command` - Send console command
  - Body: `{command: string}`

### Versions
- `GET /api/versions` - Available Minecraft versions
  - Fetches from Mojang API (cached 1 hour)

### WebSocket
- `WS /api/ws/:id` - Log streaming and status updates
  - Messages: `{type: "log"|"status", ...}`

## Database Schema

```sql
CREATE TABLE servers (
    id TEXT PRIMARY KEY,              -- UUID v4
    name TEXT NOT NULL,               -- User-provided name
    version TEXT NOT NULL,            -- e.g., "1.20.1"
    port INTEGER NOT NULL UNIQUE,     -- 25565-25600 (configurable)
    status TEXT NOT NULL,             -- stopped, starting, running, stopping
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_started_at TIMESTAMP
);
```

## Environment Variables

```bash
REALMRUNNER_PASSWORD_HASH=<bcrypt>  # Required
REALMRUNNER_MAX_RUNNING=3           # Max concurrent servers
REALMRUNNER_PORT_RANGE=25565-25600  # Allowed port range
REALMRUNNER_MEMORY_MB=2048          # Memory per server
REALMRUNNER_DATA_DIR=/data          # Data directory
```

## Key Algorithms & Logic

### Port Validation
1. Parse range from env var (e.g., "25565-25600")
2. Check if user-provided port is within range
3. Query database for existing servers with same port
4. Reject if conflict detected

### Max Running Limit
1. Count servers with status="running"
2. If count >= MAX_RUNNING, reject start request
3. Return error to frontend for display

### Graceful Shutdown
1. Send SIGTERM to process
2. Wait up to 30 seconds for clean exit
3. If still running, send SIGKILL
4. Update status to "stopped"

### Log Tailing
1. Open logs/latest.log in read mode
2. Seek to end of file
3. Use `inotify` or polling to detect new lines
4. Send new lines to WebSocket clients

### Server JAR Caching (Optional)
1. Download server.jar to /data/cache/{version}/server.jar
2. On create, check if version exists in cache
3. If yes, copy from cache instead of downloading
4. Saves bandwidth and time for duplicate versions

## Common Tasks

### Adding New API Endpoint
1. Define handler in `backend/api/handlers.go`
2. Add route in `backend/api/routes.go`
3. Apply auth middleware if needed
4. Update frontend API client in `frontend/src/api/client.js`
5. Call from relevant Vue component

### Adding Server Property
1. Update database schema with migration
2. Add field to server struct in `backend/server/db.go`
3. Update create/update handlers
4. Add field to frontend form/display

### Modifying WebSocket Messages
1. Update message struct in `backend/websocket/client.go`
2. Update sender logic in log streamer
3. Update receiver logic in `frontend/src/components/Console.vue`

## Testing Strategy

### Backend Unit Tests
- Auth middleware (password verification, JWT)
- Port validation logic
- Version fetching and caching
- Process management mocks

### Frontend Unit Tests
- Component rendering
- API client mocks
- Form validation

### Integration Tests
- Full server creation flow
- Start/stop/wipeout operations
- WebSocket connection and messages
- Auth flow end-to-end

### Manual Testing Checklist
- [ ] Create server with valid/invalid inputs
- [ ] Start multiple servers, hit max limit
- [ ] Stop server, verify graceful shutdown
- [ ] Wipeout server, verify data deleted
- [ ] View logs, verify real-time updates
- [ ] Send console commands
- [ ] Login with correct/incorrect password
- [ ] Port conflict handling

## Known Limitations

1. **No per-server memory config**: All servers get same allocation
2. **No RBAC**: Single password for all users
3. **No backups**: Wipeout is permanent
4. **Basic port conflict handling**: No auto-reassignment
5. **No resource monitoring**: No CPU/RAM usage displayed (optional feature)
6. **No crash recovery**: Server crashes require manual restart

## Security Considerations

### Password Storage
- Never log or expose password hash
- Use bcrypt with cost >= 10
- JWT secret should be generated securely (not in env var, ideally)

### Command Injection
- Validate console commands before passing to stdin
- Don't allow shell metacharacters if executing via shell
- Use Go's exec.Command with separate args (not shell string)

### Path Traversal
- Validate server IDs are UUIDs
- Never use user input directly in file paths
- Ensure data dir operations stay within /data/servers/

### Resource Exhaustion
- Enforce max running servers
- Set memory limits per process
- Consider disk quota per server (future)

## Docker Build

Multi-stage Dockerfile:
1. **Stage 1**: Build Vue frontend (node:20)
2. **Stage 2**: Build Go backend (golang:1.21)
3. **Stage 3**: Runtime (openjdk:17-slim)
   - Copy built static files from stage 1
   - Copy Go binary from stage 2
   - Install ca-certificates for HTTPS
   - Expose port 8080
   - Set ENTRYPOINT to backend binary

## Deployment Checklist

- [ ] Set strong REALMRUNNER_PASSWORD_HASH
- [ ] Configure appropriate REALMRUNNER_MAX_RUNNING
- [ ] Ensure REALMRUNNER_PORT_RANGE matches exposed ports
- [ ] Mount persistent volume to /data
- [ ] Use HTTPS reverse proxy (Nginx, Caddy, Traefik)
- [ ] Set up firewall rules for Minecraft ports
- [ ] Configure automatic backups of /data volume
- [ ] Set restart policy (unless-stopped or always)
- [ ] Monitor disk usage on /data volume

## Future Enhancements

### High Priority
- [ ] User accounts with permissions
- [ ] Per-server memory configuration
- [ ] Server.properties editor in UI
- [ ] Automatic backups/snapshots

### Medium Priority
- [ ] Plugin support (Paper, Spigot, Fabric, Forge)
- [ ] Resource usage monitoring (CPU, RAM, players)
- [ ] Scheduled restarts
- [ ] Whitelist/ops management

### Low Priority
- [ ] Server templates
- [ ] File browser for server files
- [ ] Multiple server actions (bulk stop/start)
- [ ] Discord notifications

## Debugging Tips

### Server Won't Start
- Check `docker logs` for backend errors
- Verify Java is installed in container
- Check memory available on host
- Look at server logs in /data/servers/{id}/logs/

### WebSocket Not Connecting
- Check browser console for errors
- Verify JWT token is valid
- Ensure WebSocket route is registered
- Check for CORS issues

### Database Locked
- SQLite locks with concurrent writes
- Ensure only one backend instance
- Use WAL mode for better concurrency

## References

- Mojang Version Manifest: https://launchermeta.mojang.com/mc/game/version_manifest.json
- Gin Framework: https://gin-gonic.com/docs/
- Vue 3 Docs: https://vuejs.org/guide/
- Minecraft Server Properties: https://minecraft.fandom.com/wiki/Server.properties
- WebSocket Protocol: https://datatracker.ietf.org/doc/html/rfc6455

## Version History

- **v0.1.0**: Initial design and specification (2025-10-10)

---

**Note to AI Assistants**: Always refer to `IMPLEMENTATION.md` for the complete feature specification before making changes. This file provides coding context and patterns to follow.
