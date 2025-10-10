# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package*.json ./
RUN npm install

COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.21 AS backend-builder

WORKDIR /app/backend

COPY backend/go.mod ./

COPY backend/ ./
RUN go mod download && go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -o realmrunner .

# Stage 3: Runtime
FROM eclipse-temurin:21-jre

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apt-get update && \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*

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
