# Stage 1 — build frontend
FROM node:20-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --legacy-peer-deps
COPY frontend/ ./
RUN npm run build

# Stage 2 — build backend (with frontend embedded)
FROM golang:1.25-alpine AS backend-build
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-build /app/dist ./internal/frontend/dist
RUN CGO_ENABLED=1 GOOS=linux go build -o nora ./cmd/nora && \
    CGO_ENABLED=1 GOOS=linux go build -o nora-cli ./cmd/nora-cli

# Stage 3 — final image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata sqlite
WORKDIR /app
COPY --from=backend-build /app/nora .
COPY --from=backend-build /app/nora-cli .
EXPOSE 8081
VOLUME ["/data"]
CMD ["./nora"]
