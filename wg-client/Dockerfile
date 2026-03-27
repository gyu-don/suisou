# syntax=docker/dockerfile:1.7

FROM alpine:3.21

RUN apk add --no-cache wireguard-tools iptables iproute2 bash

COPY --chmod=755 entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
