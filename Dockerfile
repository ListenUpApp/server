# Build stage 1: Build librempeg from source
# Librempeg is a fork of FFmpeg with additional codec support including Dolby AC-4
FROM debian:bookworm-slim AS librempeg-builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    git \
    nasm \
    yasm \
    pkg-config \
    libssl-dev \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# Clone and build librempeg
WORKDIR /build
RUN git clone --depth 1 https://github.com/librempeg/librempeg.git

WORKDIR /build/librempeg
RUN ./configure \
    --prefix=/usr/local \
    --enable-gpl \
    --enable-version3 \
    --enable-nonfree \
    --enable-agpl \
    --enable-openssl \
    --disable-debug \
    --disable-doc \
    --disable-ffplay \
    --disable-network \
    --enable-static \
    --disable-shared \
    && make -j$(nproc) \
    && make install

# Build stage 2: Build Go application
FROM golang:1.23-bookworm AS go-builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with static linking
ENV CGO_ENABLED=0
ENV GOEXPERIMENT=jsonv2
RUN go build -ldflags="-s -w" -o /app/listenup-server ./cmd/api

# Final stage: Runtime image
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libssl3 \
    && rm -rf /var/lib/apt/lists/*

# Copy librempeg binaries
COPY --from=librempeg-builder /usr/local/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=librempeg-builder /usr/local/bin/ffprobe /usr/local/bin/ffprobe

# Copy Go binary
COPY --from=go-builder /app/listenup-server /usr/local/bin/listenup-server

# Create non-root user
RUN useradd -m -s /bin/bash listenup

# Create directories for data
RUN mkdir -p /data/metadata /data/audiobooks /data/cache && \
    chown -R listenup:listenup /data

USER listenup

# Default environment
ENV ENV=production
ENV LOG_LEVEL=info
ENV METADATA_PATH=/data/metadata
ENV AUDIOBOOK_PATH=/data/audiobooks
ENV TRANSCODE_CACHE_PATH=/data/cache/transcode
ENV SERVER_PORT=8080
ENV FFMPEG_PATH=/usr/local/bin/ffmpeg

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["listenup-server"]
