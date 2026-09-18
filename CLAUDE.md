# RealmRunner - Development Context

This document provides context for AI assistants (like Claude) working on the RealmRunner codebase across multiple sessions.

## Project Identity

**Name**: RealmRunner
**Purpose**: Web-based Minecraft Java Edition server manager
**Repository**: git@github.com:wzin/realmrunner.git
**Target Deployment**: Docker container via Komodo, SSL terminated by Traefik
**Production URL**: https://realmrunner.ziniewicz.eu

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
│   │   ├── java.go             # Java runtime requirement + resolution
│   │   └── downloader.go       # JAR download & cache
│   ├── mcproto/
│   │   └── protocol.go         # Handshake, status ping, login disconnect
│   ├── rcon/
│   │   ├── client.go           # Source RCON protocol
│   │   └── players.go          # "list" output parsing
│   ├── sleepproxy/
│   │   └── proxy.go            # Holds a sleeping realm's port, wakes on join
│   ├── server/
│   │   ├── manager.go          # Server lifecycle
│   │   ├── process.go          # Process management
│   │   ├── control.go          # RCON provisioning and commands
│   │   ├── crash.go            # Crash detection, backoff restarts, preflight
│   │   ├── sleep.go            # Auto-sleep proxies and idle watcher
│   │   ├── diagnostics.go      # Disconnect/lag analysis from server logs
│   │   ├── memory.go           # Heap pressure from the GC log
│   │   ├── upgrade.go          # Backed-up, verified, reversible upgrades
│   │   ├── properties.go       # server.properties editing
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
├── compose.yaml                 # Docker Compose (Komodo-compatible)
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
3. Resolve the Java runtime for the server's version (see "Java Runtime Selection")
4. Fork process: `<java> -Xmx{memory}M -jar server.jar nogui`
5. Capture PID, track status
6. Stream logs via WebSocket
7. Update status to "running"

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
- `POST /api/servers/:id/command` - Send console command (RCON, falls back to stdin)
  - Body: `{command: string}`
- `POST /api/servers/:id/wake` - Start a sleeping server
- `POST /api/servers/:id/sleep` - Stop a server but keep its port held
- `PUT /api/servers/:id/autosleep` - Configure idle shutdown
  - Body: `{enabled: bool, idle_timeout_min: number}`
- `PUT /api/servers/:id/autorestart` - Configure crash restarts
  - Body: `{enabled: bool}`

### Players (RCON)
- `GET /api/servers/:id/diagnostics` - Connection health and heap pressure, read from the logs
- `PUT /api/servers/:id/heap` - Set the Java heap
  - Body: `{heap_mb: number}`
- `GET /api/servers/:id/players` - Who is online
- `POST /api/servers/:id/players/kick` - Body: `{player: string, reason?: string}`
- `POST /api/servers/:id/players/ban` - Body: `{player: string, reason?: string}`
- `POST /api/servers/:id/players/pardon` - Body: `{player: string}`
- `POST /api/servers/:id/players/op` - Body: `{player: string}`
- `POST /api/servers/:id/players/deop` - Body: `{player: string}`

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
    status TEXT NOT NULL,             -- stopped, starting, running, stopping, sleeping, crashed
    rcon_port INTEGER,                -- loopback control port (port + 10000)
    rcon_password TEXT,               -- generated per server
    auto_sleep INTEGER,               -- hold the port and sleep when empty
    idle_timeout_min INTEGER,         -- minutes empty before sleeping
    internal_port INTEGER,            -- where the server listens behind the proxy
    auto_restart INTEGER,             -- restart after a crash
    last_exit_code INTEGER,           -- how the process last ended
    last_error TEXT,                  -- why it crashed, shown in the UI
    heap_mb INTEGER,                  -- per-server Java heap; 0 uses the default
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_started_at TIMESTAMP
);
```

## Environment Variables

```bash
REALMRUNNER_PASSWORD_HASH=<bcrypt>  # Required
REALMRUNNER_JWT_SECRET=<secret>     # Required
REALMRUNNER_MAX_RUNNING=3           # Max concurrent servers
REALMRUNNER_PORT_RANGE=25565-25600  # Allowed port range
REALMRUNNER_MEMORY_MB=2048          # Memory per server
REALMRUNNER_DATA_DIR=/data          # Data directory
REALMRUNNER_BASE_URL=realmrunner.ziniewicz.eu  # Display domain
```

## Key Algorithms & Logic

### JVM Heap and GC Flags

Each server may set its own heap (`heap_mb`, falling back to
`REALMRUNNER_MEMORY_MB`), and every server starts with G1 tuned the way
Minecraft operators have converged on (Aikar's flags), with the new-generation
sizes and region size scaling at a 12 GB heap. Default GC settings give
multi-second stop-the-world pauses on a populated server, which appear as
"Can't keep up!" in the log and as players timing out, because the server misses
keep-alives while it is paused.

### Judging Memory Pressure

Resident memory does not answer "does this server need more RAM": RealmRunner
starts the JVM with `-Xms` equal to `-Xmx`, so the heap is committed at startup
and the process shows the same size whether it is busy or idle. A 2 GB heap
looks like roughly 2.7 GB resident from the first minute.

Every server therefore writes `logs/gc.log` (`-Xlog:gc`, 3 files of 8 MB), and
`ReadMemoryReport` parses it:

- **Live set**: heap occupancy right after a collection - what the world needs
- **Peak used**: occupancy before a collection
- **Full GCs** and the **longest pause**: a server short of heap collects
  constantly and stops the world while it does

Over ~85% live, or any full GC, means it is short of heap; the report suggests
roughly double the live set, rounded to a whole gigabyte.

`RequiredContainerMB` is the memory a container needs for a heap: heap × 4/3 +
256 MB, since metaspace, GC structures, thread stacks, the code cache and direct
buffers sit outside the heap. A container limit set to the heap size gets the
JVM killed by the kernel rather than reporting an out-of-memory error.

### Java Runtime Selection

Minecraft versions require different Java releases, and running a jar on a too-old JVM makes the
server exit immediately with `UnsupportedClassVersionError`:

| Minecraft version | Java |
|---|---|
| 26.x (year-based scheme) | 25 |
| 1.20.5 - 1.21.x | 21 |
| 1.17 - 1.20.4 | 17 |
| 1.16.5 and older | 8 |

1. Ask upstream first: Mojang's version manifest reports `javaVersion.majorVersion`, PaperMC's v3
   API reports `java.version.minimum`
2. Fall back to the table above (`minecraft.RequiredJavaMajor`) when upstream is unreachable
3. Resolve a binary (`minecraft.JavaCommand`): `$REALMRUNNER_JAVA_<major>`, then
   `/opt/java/<major>/bin/java` or `/usr/lib/jvm/*`, then the oldest installed runtime that is new
   enough, then plain `java` from PATH

### RCON Control Channel

Every server gets an RCON endpoint on loopback (`port + 10000`) with a random
password, written into `server.properties` at start and stored in the database.
It is never published by the container.

RCON is preferred over the stdin pipe because it confirms delivery, returns the
server's answer, and works for a server this process did not start. `SendCommand`
falls back to stdin when RCON is unavailable.

It backs the live player list (`list`), kick/ban/pardon/op/deop, and the
auto-sleep idle check.

### Auto-Sleep and Wake-on-Join

With auto-sleep enabled, the Minecraft process moves to an internal port
(`port + 20000`) and `sleepproxy` owns the public one, so players always use the
same address:

1. The proxy answers the server list ping itself while the realm is down, with a
   "sleeping" MOTD; a ping never starts the server
2. A login attempt calls `Wake`, which starts the server, then holds the player's
   connection, dials the internal port once it answers, replays the handshake and
   pipes the connection through
3. If the server is not up within 90s the player gets a "still starting,
   reconnect in a moment" disconnect rather than a hang
4. The idle watcher asks each running realm over RCON how many players are on,
   every 60s, and sleeps it once it has been empty for `idle_timeout_min`

A server that is not running is `sleeping` rather than `stopped` whenever its
port is still held. Toggling auto-sleep requires the server to be stopped,
because the Minecraft process has to move between ports.

### Crash Recovery

`monitorProcess` distinguishes an operator-initiated stop from a crash using the
process's `stopRequested` flag. A crash records the exit code and a cause read
from the tail of the log (too-old Java runtime, out of memory, port in use, EULA)
and, when `auto_restart` is on, restarts with a growing delay (10s, 30s, 60s,
2m, 5m) before giving up. A server that stays up for 10 minutes gets a clean
slate.

`preflightJava` refuses to start a server whose version needs a newer runtime
than any installed, turning what used to be an instant silent exit into a clear
message.

### Safe Upgrades

`UpgradeServer` checks the Java requirement, takes a world backup, sets the old
jar aside, downloads the new one, starts the server and waits for it to answer on
its control port. Any failure restores the previous jar and version; the backup
is kept either way.

### Upstream APIs

| Purpose | Endpoint |
|---|---|
| Vanilla versions/downloads | `https://piston-meta.mojang.com/mc/game/version_manifest_v2.json` |
| Paper versions/builds | `https://fill.papermc.io/v3/projects/paper` (v2 was sunset) |
| Purpur versions/builds | `https://api.purpurmc.org/v2/purpur` |
| Username -> UUID | `https://api.minecraftservices.com/minecraft/profile/lookup/name/{name}` (legacy `api.mojang.com` used as fallback) |
| Mods | `https://api.modrinth.com/v2` |

`go test ./minecraft/ -run TestLive` with `REALMRUNNER_LIVE_TESTS=1` checks these against the real
APIs; CI runs it weekly so a sunset endpoint surfaces before users hit it.

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

### Log Capture

The Minecraft server writes `logs/latest.log` itself through log4j, so
RealmRunner captures the process output to `logs/console.log` instead. Two
writers appending to one file interleave half-lines and make the log unusable.

`console.log` is a superset (it also holds JVM-level output such as
`UnsupportedClassVersionError`), so readers prefer it and fall back to
`latest.log` for servers created before this. The previous run is kept as
`console.log.1`, and stdout and stderr are read concurrently - reading them in
sequence held back every stack trace until the process exited.

### Connection Diagnostics

`AnalyseLogs` summarises a server's log into disconnect reasons with plain-word
explanations, per-player drop counts with addresses, lag warnings with the worst
stall, and restart counts. It exists because "connection lost" has several
distinct causes that look alike in a raw log:

| Log reason | What it means |
|---|---|
| `Disconnected` | The connection dropped with no goodbye: a network path problem, not a server decision |
| `Timed out` | No keep-alive answer for 30s: a broken path, or the server stalled long enough to miss them |
| `You logged in from another location` | The account reconnected while the server still held a dead session |

Several players sharing one address is called out explicitly, because drops
affecting only that address point at that link rather than the server.

### Log Tailing
1. Open logs/console.log in read mode
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

1. **Roles, not per-realm permissions**: owner/admin/operator/viewer
2. **Wipeout is permanent**: backups exist, but wipeout does not take one
3. **Basic port conflict handling**: No auto-reassignment
4. **Auto-sleep needs a restart to toggle**: the server changes port

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
1. **Stage 1**: Build Vue frontend (node:22-alpine)
2. **Stage 2**: Build Go backend (golang:1.25)
3. **Stage 3**: Runtime (eclipse-temurin:25-jre, with Java 21 copied to /opt/java/21)
   - Ship both JREs side by side: Minecraft 26.x requires Java 25, 1.20.5-1.21.x run on Java 21
   - Copy built static files from stage 1
   - Copy Go binary from stage 2
   - Install ca-certificates for HTTPS
   - Expose port 8080
   - Set CMD to backend binary

## Deployment

RealmRunner is deployed via **Komodo** which manages the stack from this git repo.
SSL is terminated by **Traefik** from the homecloud stack (`traefik_proxy` Docker network).

The `compose.yaml` includes:
- Traefik labels routing `realmrunner.ziniewicz.eu` to container port 8080
- `traefik_proxy` external network for Traefik connectivity
- Minecraft ports 25565-25600 exposed directly
- Build config with `network: host` to work around Docker DNS issues
- Port 8080 commented out (only needed for local dev)

Environment variables (`REALMRUNNER_PASSWORD_HASH`, `REALMRUNNER_JWT_SECRET`) are set in Komodo's stack configuration. Use unescaped bcrypt hashes in Komodo's UI.

### Deployment Checklist

- [ ] Set strong REALMRUNNER_PASSWORD_HASH in Komodo
- [ ] Set REALMRUNNER_JWT_SECRET in Komodo
- [ ] Configure appropriate REALMRUNNER_MAX_RUNNING
- [ ] Ensure REALMRUNNER_PORT_RANGE matches exposed ports
- [ ] Mount persistent volume to /data
- [ ] Set up firewall rules for Minecraft ports
- [ ] Configure automatic backups of /data volume
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
- **v0.2.0**: Komodo deployment with Traefik SSL termination (2026-03-11)
- **v1.1.0**: Tag of the last main before 2.0 (Java 21 only, Paper API v2)
- **v2.0.0**: Minecraft 26.x support (Java 25 runtime + per-version Java selection), PaperMC v3
  API migration, modern Mojang profile lookup, backend test suite (2026-09-18)
- **v2.1.0**: RCON control channel with live player management, auto-sleep with wake-on-join,
  crash recovery with backoff restarts, backed-up and reversible upgrades, metrics history fix
  for the 24h/7d/30d ranges (2026-09-18)

---

**Note to AI Assistants**: Always refer to `IMPLEMENTATION.md` for the complete feature specification before making changes. This file provides coding context and patterns to follow.
