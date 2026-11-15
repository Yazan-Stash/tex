# Build and test environment for tex. Go never runs on the host.
FROM golang:1.25-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends tmux \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
