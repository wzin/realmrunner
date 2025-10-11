# SSL Setup for RealmRunner

This guide shows how to add SSL/HTTPS to RealmRunner using Caddy as a reverse proxy. No source code changes or rebuilding required!

## Quick Start

### Option 1: Let's Encrypt (Automatic SSL)

1. Copy the example environment file:
```bash
cp .env.ssl.example .env
```

2. Edit `.env` and set your domain:
```env
CADDY_DOMAIN=minecraft.example.com
REALMRUNNER_PASSWORD_HASH=$2b$12$YourHashHere
```

3. Start the services:
```bash
docker compose -f docker-compose-ssl.yml up -d
```

That's it! Caddy will automatically obtain and renew SSL certificates from Let's Encrypt.

### Option 2: Custom SSL Certificate

1. Copy the example environment file:
```bash
cp .env.ssl.example .env
```

2. Place your certificate files:
```bash
mkdir -p certs
cp /path/to/your/fullchain.pem ./certs/cert.pem
cp /path/to/your/privkey.pem ./certs/privkey.pem
```

3. Edit `.env` and configure:
```env
CADDY_DOMAIN=minecraft.example.com
REALMRUNNER_PASSWORD_HASH=$2b$12$YourHashHere

# Point to your certificate files
CADDY_TLS_CERT=/etc/caddy/certs/cert.pem
CADDY_TLS_KEY=/etc/caddy/certs/privkey.pem
```

4. Start the services:
```bash
docker compose -f docker-compose-ssl.yml up -d
```

## Environment Variables

All configuration is done through environment variables. See `.env.ssl.example` for the complete list.

### Required Variables
- `CADDY_DOMAIN` - Your domain name
- `REALMRUNNER_PASSWORD_HASH` - Password hash (generate with `./generate-password.sh`)

### SSL Configuration
- `CADDY_TLS_CERT` - Path to certificate (leave empty for Let's Encrypt)
- `CADDY_TLS_KEY` - Path to private key (leave empty for Let's Encrypt)

### Optional Settings
- `REALMRUNNER_IMAGE` - Docker image to use (default: ghcr.io/wzin/realmrunner:main)
- `MINECRAFT_PORT_RANGE` - Port range for Minecraft servers (default: 25565-25600)
- `DATA_DIR` - Data directory (default: ./data)
- And more - see `.env.ssl.example`

## Using Pre-built Image vs Building Locally

### Pre-built Image (Recommended)
```env
REALMRUNNER_IMAGE=ghcr.io/wzin/realmrunner:main
```

### Build Locally
```bash
# First build the image
docker build -t realmrunner:local .

# Then use it in .env
REALMRUNNER_IMAGE=realmrunner:local
```

## Switching from Non-SSL to SSL

If you're already running RealmRunner without SSL:

1. Stop the current deployment:
```bash
docker compose down
```

2. Follow the SSL setup steps above

3. Start with SSL:
```bash
docker compose -f docker-compose-ssl.yml up -d
```

Your data in `./data` will be preserved.

## Troubleshooting

### Port 80/443 Already in Use
Change the ports in `.env`:
```env
HTTP_PORT=8080
HTTPS_PORT=8443
```

### Certificate Not Working
- Ensure DNS points to your server
- Check Caddy logs: `docker logs caddy`
- For Let's Encrypt: Ensure port 80 is accessible from the internet

### Custom Certificate Issues
- Check file permissions: certificates should be readable
- Verify certificate chain is complete (cert + intermediate)
- Check paths in `.env` match mounted files

## Security Notes

- The web UI is only accessible through HTTPS (port 443)
- HTTP (port 80) automatically redirects to HTTPS
- Minecraft game ports remain on 25565-25600 (direct access)
- RealmRunner backend is not exposed directly (only through Caddy)

## File Structure After Setup

```
realmrunner/
├── .env                    # Your configuration (create from .env.ssl.example)
├── docker-compose-ssl.yml  # Docker Compose with SSL
├── Caddyfile              # Caddy configuration (uses env vars)
├── data/                  # Minecraft server data
├── caddy_data/            # Caddy certificates (Let's Encrypt)
├── caddy_config/          # Caddy config
└── certs/                 # Your custom certificates (if using)
    ├── cert.pem
    └── privkey.pem
```