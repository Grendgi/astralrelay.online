FROM postgres:16-alpine
COPY replica-entrypoint.sh /replica-entrypoint.sh
RUN chmod +x /replica-entrypoint.sh
ENTRYPOINT ["/replica-entrypoint.sh"]
