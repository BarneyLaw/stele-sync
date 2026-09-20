FROM busybox:1.37.0-musl AS shell
FROM dxflrs/garage:v2.3.0
# The official Garage image has no shell. Add a static BusyBox solely for
# local bootstrap/health checks; the production image is unaffected.
COPY --from=shell /bin/busybox /bin/busybox
COPY deploy/dev/garage.toml /etc/garage.toml
COPY deploy/dev/garage-entrypoint.sh /garage-entrypoint.sh
ENTRYPOINT ["/bin/busybox", "sh", "/garage-entrypoint.sh"]
