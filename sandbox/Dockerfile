# syntax=docker/dockerfile:1.7

FROM ghcr.io/openclaw/openclaw:latest

USER root

RUN --mount=type=cache,id=suisou-sandbox-apt-cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=suisou-sandbox-apt-lists,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends gosu

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /app

ENTRYPOINT ["entrypoint.sh"]
CMD ["node", "openclaw.mjs", "gateway", "--bind", "lan", "--port", "18789", "--allow-unconfigured"]
