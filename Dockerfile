# Stage 1: Build Go binary
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o pentaract ./cmd/pentaract

# Stage 2: Build UI
FROM public.ecr.aws/docker/library/node:22-slim AS ui

WORKDIR /app
COPY ui/package.json ui/pnpm-lock.yaml* ./
RUN npm install -g pnpm && pnpm install

COPY ui/ .
ENV VITE_API_BASE=/api
RUN pnpm run build

# Stage 3: Runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/pentaract /pentaract
COPY --from=ui /app/dist /ui/dist

ENTRYPOINT ["/pentaract"]
