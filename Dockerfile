FROM node:22-alpine AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS builder

# create nobody user/group for scratch runtime
RUN echo "nobody:x:65534:65534:nobody:/:/sbin/nologin" > /etc/passwd && \
    echo "nobody:x:65534:" > /etc/group

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist internal/handler/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o oci-helper ./cmd/server
RUN mkdir -p /tmp/oci-helper/keys && chmod 700 /tmp/oci-helper/keys

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /app/oci-helper /oci-helper
COPY --from=builder /tmp/oci-helper/keys /app/oci-helper/keys
USER 65534:65534
EXPOSE 8818
CMD ["/oci-helper"]
