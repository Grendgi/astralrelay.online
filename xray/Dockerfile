FROM teddysun/xray:latest
RUN apk add --no-cache openssl
COPY xray/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
