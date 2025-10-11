#!/bin/bash

echo "==================================="
echo "RealmRunner SSL Setup"
echo "==================================="
echo ""
echo "Choose your SSL option:"
echo "1) Let's Encrypt (automatic, recommended)"
echo "2) Custom certificate (your own SSL cert)"
echo ""
read -p "Enter choice [1-2]: " choice

case $choice in
    1)
        echo ""
        echo "Setting up Let's Encrypt with Caddy..."
        cp Caddyfile.letsencrypt Caddyfile

        echo ""
        read -p "Enter your domain (e.g., minecraft.example.com): " domain
        sed -i "s/your-domain.com/$domain/g" Caddyfile
        sed -i "s/your-domain.com/$domain/g" docker-compose-ssl.yml

        echo ""
        echo "✅ Configuration ready for Let's Encrypt!"
        echo ""
        echo "To start with SSL:"
        echo "  docker compose -f docker-compose-ssl.yml up -d"
        echo ""
        echo "Caddy will automatically obtain and renew certificates."
        ;;

    2)
        echo ""
        echo "Setting up custom certificate..."
        cp Caddyfile.custom Caddyfile

        # Create certs directory
        mkdir -p certs

        echo ""
        echo "Please place your certificate files in the ./certs directory:"
        echo "  - cert.pem (your certificate + intermediate chain)"
        echo "  - privkey.pem (your private key)"
        echo ""
        echo "Example commands:"
        echo "  cp /path/to/your/fullchain.pem ./certs/cert.pem"
        echo "  cp /path/to/your/privkey.pem ./certs/privkey.pem"
        echo ""

        read -p "Enter your domain (e.g., minecraft.example.com): " domain
        sed -i "s/your-domain.com/$domain/g" Caddyfile
        sed -i "s/your-domain.com/$domain/g" docker-compose-ssl.yml

        echo ""
        echo "After copying certificates, start with:"
        echo "  docker compose -f docker-compose-ssl.yml up -d"
        ;;

    *)
        echo "Invalid choice. Exiting."
        exit 1
        ;;
esac

echo ""
echo "==================================="
echo "Additional Notes:"
echo "==================================="
echo "- Make sure ports 80 and 443 are open in your firewall"
echo "- DNS should point to this server"
echo "- The web UI will be available at https://$domain"
echo "- Minecraft servers remain on ports 25565-25600"
echo ""
echo "To stop the old deployment first:"
echo "  docker compose down"
echo ""