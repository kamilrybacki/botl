FROM node:22-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN npm install -g @anthropic-ai/claude-code

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

COPY botl-postrun /usr/local/bin/botl-postrun
RUN chmod +x /usr/local/bin/botl-postrun

WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
