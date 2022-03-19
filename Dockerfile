ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="Ben Kochie <superq@gmail.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/pgbouncer_exporter /bin/chrony_exporter
COPY LICENSE                                /LICENSE

USER       nobody
ENTRYPOINT ["/bin/chrony_exporter"]
EXPOSE     9123
