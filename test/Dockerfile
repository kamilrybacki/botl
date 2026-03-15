FROM golang:1.24-bookworm

# Install Docker CE CLI + daemon for Docker-in-Docker
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl gnupg iptables supervisor && \
    install -m 0755 -d /etc/apt/keyrings && \
    curl -fsSL https://download.docker.com/linux/debian/gpg | \
        gpg --dearmor -o /etc/apt/keyrings/docker.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
        https://download.docker.com/linux/debian bookworm stable" \
        > /etc/apt/sources.list.d/docker.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        docker-ce docker-ce-cli containerd.io && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Cache Go module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy entire source tree
COPY . .

COPY test/run-tests.sh /run-tests.sh
RUN chmod +x /run-tests.sh

ENTRYPOINT ["/run-tests.sh"]
