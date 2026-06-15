FROM node:22-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist internal/handler/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server
RUN mkdir -p /app/oci-helper/keys && chmod 777 /app/oci-helper/keys

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /app/oci-helper /oci-helper
COPY --from=builder /app/oci-helper/keys /app/oci-helper/keys
USER nobody
EXPOSE 8818
CMD ["/oci-helper"]
