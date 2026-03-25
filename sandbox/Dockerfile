# syntax=docker/dockerfile:1.7

FROM ghcr.io/openclaw/openclaw:latest

USER root

RUN --mount=type=cache,id=suisou-sandbox-apt-cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=suisou-sandbox-apt-lists,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends gosu

COPY --chmod=755 entrypoint.sh /usr/local/bin/suisou-entrypoint.sh

ENTRYPOINT ["suisou-entrypoint.sh"]
CMD ["node", "openclaw.mjs", "gateway", "--allow-unconfigured", "--bind", "lan"]
