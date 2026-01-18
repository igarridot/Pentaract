############################################################################################
####  SERVER
############################################################################################

FROM rust:alpine AS chef
RUN apk add --no-cache musl-dev pkgconfig openssl-dev openssl-libs-static
RUN cargo install cargo-chef
WORKDIR /app

FROM chef AS planner
COPY ./pentaract .
RUN cargo chef prepare --recipe-path recipe.json

FROM chef AS builder
COPY --from=planner /app/recipe.json recipe.json
# Build dependencies - this is the caching Docker layer!
RUN cargo chef cook --release --recipe-path recipe.json
# Build application
COPY ./pentaract .
RUN cargo build --release

############################################################################################
####  UI
############################################################################################

FROM node:21-slim AS ui
WORKDIR /app
COPY ./ui .
RUN npm install -g pnpm
RUN pnpm i
ENV VITE_API_BASE=/api
RUN pnpm run build

############################################################################################
####  RUNNING
############################################################################################

# We do not need the Rust toolchain to run the binary!
FROM alpine:latest AS runtime
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/target/release/pentaract /
COPY --from=ui /app/dist /ui
ENTRYPOINT ["/pentaract"]
