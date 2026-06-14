# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

# prepare passwd/group for scratch
RUN echo "nobody:x:65534:65534:nobody:/:/sbin/nologin" > /etc/passwd && \
    echo "nobody:x:65534:" > /etc/group

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /oci-helper ./cmd/server

FROM scratch

COPY --from=builder /oci-helper /oci-helper
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /etc/passwd /etc/group /etc/

USER 65534
EXPOSE 8818
ENV PORT=8818
ENTRYPOINT ["/oci-helper"]
