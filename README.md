# RealmRunner

A web-based Minecraft Java Edition server manager packaged as a Docker container. Easily create, manage, and control multiple Minecraft servers through a simple web interface.

## Features

- 🎮 **Multiple Servers**: Create and manage multiple Minecraft servers
- 🔄 **Version Selection**: Choose from official Minecraft Java Edition releases
- 🎛️ **Server Controls**: Start, stop, and wipeout servers with one click
- 📊 **Real-time Logs**: View server logs as they happen
- 💻 **Console Access**: Send commands directly to running servers
- 🔒 **Password Protected**: Secure access with password authentication
- 🐳 **Docker Ready**: Single container with all dependencies included

## Quick Start

### Prerequisites

- Docker and Docker Compose installed
- Ports available for web UI (default: 8080) and Minecraft servers (default: 25565-25600)

### Running with Docker Compose

1. Create a `docker-compose.yml` file:

```yaml
version: '3.8'
services:
  realmrunner:
    image: realmrunner:latest
    ports:
      - "8080:8080"                    # Web UI
      - "25565-25600:25565-25600"      # Minecraft servers
    volumes:
      - ./data:/data                   # Persistent server data
    environment:
      REALMRUNNER_PASSWORD_HASH: "$2a$10$YourBcryptHashHere"
      REALMRUNNER_MAX_RUNNING: 3
      REALMRUNNER_PORT_RANGE: "25565-25600"
      REALMRUNNER_MEMORY_MB: 2048
    restart: unless-stopped
```

2. Generate a password hash (see Configuration section below)

3. Start the container:

```bash
docker compose up -d
```

4. Open your browser to `http://localhost:8080`

5. Login with your password and start creating servers!

## Configuration

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `REALMRUNNER_PASSWORD_HASH` | Bcrypt hash of your password | - | ✅ |
| `REALMRUNNER_MAX_RUNNING` | Maximum number of running servers | 3 | ❌ |
| `REALMRUNNER_PORT_RANGE` | Port range for Minecraft servers | 25565-25600 | ❌ |
| `REALMRUNNER_MEMORY_MB` | Memory allocation per server (MB) | 2048 | ❌ |
| `REALMRUNNER_DATA_DIR` | Data directory path | /data | ❌ |

### Generating a Password Hash

**Using the included script (recommended):**

```bash
./generate-password.sh yourpassword
```

This uses Docker (no local dependencies needed) and outputs:
- The bcrypt hash
- The exact line to add to `docker-compose.yml`

**Manual methods:**

```bash
# Using Docker with Python
docker run --rm python:3.11-slim sh -c \
  "pip install -q bcrypt && python -c 'import bcrypt; print(bcrypt.hashpw(b\"yourpassword\", bcrypt.gensalt()).decode())'"

# Using online tool (less secure for production)
# Visit: https://bcrypt-generator.com/ (set cost to 10)
```

## Usage

### Creating a Server

1. Click the **"Create Server"** button
2. Enter a name for your server
3. Select a Minecraft version from the dropdown
4. Specify a port (within configured range)
5. Click **"Create"**

The server will be downloaded, configured, and ready to start.

### Starting a Server

Click the **"Start"** button on a server card. The server will transition through:
- **Starting**: Server is launching
- **Running**: Server is online and accepting connections

Connect to your server using the displayed port: `your-server-ip:PORT`

### Stopping a Server

Click the **"Stop"** button. The server will gracefully shut down (30 second timeout).

### Viewing Logs & Console

1. Click on a server card to open the console
2. View real-time logs as they stream
3. Enter commands in the input field (e.g., `/say Hello!`)
4. Press Enter to send commands

### Wiping Server Data

Click the **"Wipeout"** button to permanently delete all server data (world, logs, configs). This action cannot be undone.

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
- RealmRunner uses password-based authentication
- All users share the same password (no user accounts)
- HTTPS is **strongly recommended** for production (use a reverse proxy like Nginx or Caddy)

### Network Security
- Only expose necessary ports from Docker
- Use firewall rules to restrict Minecraft port access
- Consider VPN for administrative access
- Change default ports if exposed to the internet

### Best Practices
- Use a strong password (16+ characters)
- Regularly backup the `/data` volume
- Monitor resource usage to prevent DoS
- Keep Docker image updated

## Troubleshooting

### Server Won't Start
- Check available memory on host
- Verify port isn't already in use
- Check logs in the console view
- Ensure Java 17+ is in the Docker image

### Can't Connect to Server
- Verify server status is "Running"
- Check firewall rules allow the port
- Ensure port is correctly mapped in Docker
- Verify client Minecraft version matches server

### Password Not Working
- Verify bcrypt hash is correctly set in environment
- Check hash doesn't have quotes or escaping issues
- Regenerate hash and update environment

### Out of Memory
- Reduce `REALMRUNNER_MEMORY_MB` per server
- Reduce `REALMRUNNER_MAX_RUNNING`
- Add more RAM to host
- Monitor resource usage per server

## Building from Source

```bash
# Clone repository
git clone git@github.com:wzin/realmrunner.git
cd realmrunner

# Build Docker image
docker build -t realmrunner:latest .

# Run
docker compose up -d
```

## Development

See `IMPLEMENTATION.md` for detailed implementation specifications.

See `CLAUDE.md` for development context and architecture decisions.

## License

MIT License - see LICENSE file for details

## Support

- Issues: https://github.com/wzin/realmrunner/issues
- Pull Requests: https://github.com/wzin/realmrunner/pulls

## Roadmap

- [ ] User accounts and permissions
- [ ] Automatic backups
- [ ] server.properties editor
- [ ] Plugin support (Paper, Spigot, Fabric)
- [ ] Resource usage alerts
- [ ] Whitelist/ops management UI
