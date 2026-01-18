############################################################################################
####  SERVER
############################################################################################

# Using the `rust-musl-builder` as base image, instead of
# the official Rust toolchain
FROM clux/muslrust:stable AS chef
USER root
RUN cargo install cargo-chef
WORKDIR /app

FROM chef AS planner
COPY ./pentaract .
RUN cargo chef prepare --recipe-path recipe.json

FROM chef AS builder

# BuildKit provides TARGETARCH automatically (amd64 or arm64)
ARG TARGETARCH

# Map Docker's architecture names to Rust target triples
RUN case "${TARGETARCH}" in \
        "amd64") echo "x86_64-unknown-linux-musl" > /tmp/rust_target ;; \
        "arm64") echo "aarch64-unknown-linux-musl" > /tmp/rust_target ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac

# Add ARM64 target if needed
RUN if [ "${TARGETARCH}" = "arm64" ]; then \
        rustup target add aarch64-unknown-linux-musl; \
    fi

COPY --from=planner /app/recipe.json recipe.json

# Build dependencies - this is the caching Docker layer!
RUN cargo chef cook --release --target $(cat /tmp/rust_target) --recipe-path recipe.json

# Build application
COPY ./pentaract .
RUN cargo build --target $(cat /tmp/rust_target) --release

# Move binary to a known location regardless of architecture
RUN cp /app/target/$(cat /tmp/rust_target)/release/pentaract /app/pentaract-binary

############################################################################################
####  UI
############################################################################################

FROM node:21-slim AS ui
WORKDIR /app
COPY ./ui .
RUN npm install -g pnpm
RUN pnpm i
ENV VITE_API_BASE /api
RUN pnpm run build

############################################################################################
####  RUNNING
############################################################################################

# We do not need the Rust toolchain to run the binary!
FROM scratch AS runtime
COPY --from=builder /app/pentaract-binary /pentaract
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=ui /app/dist /ui
ENTRYPOINT ["/pentaract"]
