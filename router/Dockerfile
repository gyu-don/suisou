# syntax=docker/dockerfile:1.7

FROM mitmproxy/mitmproxy:latest

USER root
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
USER mitmproxy

COPY addon.py /opt/suisou/addon.py

EXPOSE 51820/udp

ENTRYPOINT ["mitmdump", "--set", "confdir=/home/mitmproxy/.mitmproxy", \
            "--set", "ssl_verify_upstream_trusted_ca=/etc/ssl/certs/ca-certificates.crt", \
            "--set", "flow_detail=0", \
            "-s", "/opt/suisou/addon.py", \
            "--mode", "wireguard"]
