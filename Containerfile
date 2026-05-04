# ── Stage 1: Go server ────────────────────────────────────────────────────────
FROM docker.io/library/golang:1.26.1-bookworm AS go-builder
WORKDIR /srv
COPY server/ .
RUN CGO_ENABLED=0 go build -o incplot-server .


# ── Stage 2: incplot (C++) ────────────────────────────────────────────────────
FROM ubuntu:24.04 AS builder

# Ubuntu 24.04 ships CMake 3.28; project requires 3.30+, so we fetch the binary.
ARG CMAKE_VERSION=4.3.0
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    ninja-build \
    gcc-14 \
    g++-14 \
    git \
    pkg-config \
    libharfbuzz-dev \
    libfontconfig1-dev \
    libssl-dev \
    patch \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL "https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}-linux-x86_64.tar.gz" \
       | tar -xz -C /opt \
    && ln -sf "/opt/cmake-${CMAKE_VERSION}-linux-x86_64/bin/cmake" /usr/local/bin/cmake

WORKDIR /src
COPY . .

RUN cmake -G Ninja \
      -DCMAKE_BUILD_TYPE=Release \
      -DCMAKE_C_COMPILER=gcc-14 \
      -DCMAKE_CXX_COMPILER=g++-14 \
      -B build \
    && cmake --build build -j$(nproc) \
    && cmake --install build --prefix /usr/local


# ── Stage 3: runtime ──────────────────────────────────────────────────────────
FROM ubuntu:24.04

ARG DUCKDB_VERSION=1.5.0

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libarchive13t64 \
    libharfbuzz0b \
    libharfbuzz-subset0 \
    libfontconfig1 \
    libssl3 \
    curl \
    fontconfig \
    unzip \
    xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSLo /tmp/duckdb.zip "https://github.com/duckdb/duckdb/releases/download/v${DUCKDB_VERSION}/duckdb_cli-linux-amd64.zip" \
    && unzip /tmp/duckdb.zip duckdb -d /usr/local/bin \
    && rm /tmp/duckdb.zip \
    && chmod +x /usr/local/bin/duckdb \
    && mkdir -p /usr/local/share/fonts/adwaita-mono \
    && curl -fsSL https://download.gnome.org/sources/adwaita-fonts/50/adwaita-fonts-50.0.tar.xz \
       | tar -xJ --strip-components=2 -C /usr/local/share/fonts/adwaita-mono \
              adwaita-fonts-50.0/mono/AdwaitaMono-Regular.ttf \
              adwaita-fonts-50.0/mono/AdwaitaMono-Bold.ttf \
              adwaita-fonts-50.0/mono/AdwaitaMono-Italic.ttf \
              adwaita-fonts-50.0/mono/AdwaitaMono-BoldItalic.ttf \
    && mkdir -p /usr/local/share/fonts/jetbrains-mono \
    && curl -fsSL https://github.com/ryanoasis/nerd-fonts/releases/download/v3.4.0/JetBrainsMono.tar.xz \
       | tar -xJ -C /usr/local/share/fonts/jetbrains-mono \
    && mkdir -p /usr/local/share/fonts/unscii \
    && curl -fsSLo /usr/local/share/fonts/unscii/unscii-16-full.ttf \
            http://viznut.fi/unscii/unscii-16-full.ttf \
    && curl -fsSLo /usr/local/share/fonts/unscii/unscii-16-full.woff \
            http://viznut.fi/unscii/unscii-16-full.woff \
    && fc-cache -f

COPY --from=builder    /usr/local/bin/incplot       /usr/local/bin/incplot
COPY --from=builder    /usr/local/share/incplot     /usr/local/share/incplot
COPY --from=go-builder /srv/incplot-server          /usr/local/bin/incplot-server

EXPOSE 8080
ENTRYPOINT ["incplot-server"]
