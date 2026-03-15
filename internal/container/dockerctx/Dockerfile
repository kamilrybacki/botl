FROM node:22-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends git ca-certificates iptables gosu && \
    rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code@latest

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

COPY botl-postrun /usr/local/bin/botl-postrun
RUN chmod +x /usr/local/bin/botl-postrun

# Create non-root user for workspace operations
RUN useradd -m -s /bin/sh botl && \
    mkdir -p /workspace /output && \
    chown botl:botl /workspace /output

# Entrypoint runs as root for iptables, then drops to botl user via gosu
WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
