# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package*.json ./
RUN npm install

COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.25 AS backend-builder

WORKDIR /app/backend

COPY backend/go.mod ./

COPY backend/ ./
RUN go mod download && go mod tidy

# Stamped by CI from the git tag; a plain build reports "dev".
ARG VERSION=dev
ARG COMMIT=""
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X github.com/wzin/realmrunner/version.Version=${VERSION} -X github.com/wzin/realmrunner/version.Commit=${COMMIT}" \
    -o realmrunner .

# Stage 3: Runtime
#
# Minecraft 26.x requires Java 25, while 1.20.5-1.21.x run on Java 21, so the
# image ships both runtimes side by side under /opt/java/<major> and the
# backend picks the right one per server version.
FROM eclipse-temurin:25-jre

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apt-get update && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Java 25 (default runtime of this image) + Java 21 for older Minecraft versions
COPY --from=eclipse-temurin:21-jre /opt/java/openjdk /opt/java/21
RUN ln -s /opt/java/openjdk /opt/java/25

# Copy built backend binary
COPY --from=backend-builder /app/backend/realmrunner /app/realmrunner

# Copy built frontend static files
COPY --from=frontend-builder /app/backend/dist /app/dist

# Create data directory
RUN mkdir -p /data

# Expose ports
EXPOSE 8080

# Set environment variables
ENV GIN_MODE=release

# Run the application
CMD ["/app/realmrunner"]
