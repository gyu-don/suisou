# syntax=docker/dockerfile:1.7

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,id=suisou-sandbox-apt-cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,id=suisou-sandbox-apt-lists,target=/var/lib/apt,sharing=locked \
    apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      curl \
      git \
      gosu \
      jq \
      python3 \
      ripgrep

RUN useradd --create-home --shell /bin/bash sandbox

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /home/sandbox

ENTRYPOINT ["entrypoint.sh"]
CMD ["sleep", "infinity"]
