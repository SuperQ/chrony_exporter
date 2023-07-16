# Prometheus Chrony Exporter

[![Build Status](https://circleci.com/gh/SuperQ/chrony_exporter/tree/main.svg?style=svg)](https://circleci.com/gh/SuperQ/chrony_exporter/tree/main)
[![Docker Repository on Quay](https://quay.io/repository/superq/chrony-exporter/status "Docker Repository on Quay")](https://quay.io/repository/superq/chrony-exporter)

This is a [Prometheus Exporter](https://prometheus.io) for [Chrony NTP](https://chrony.tuxfamily.org/).

## Installation

For most use-cases, simply download the [the latest
release](https://github.com/superq/chrony_exporter/releases).

### Building from source

You need a Go development environment. Then, simply run `make` to build the
executable:

    make

This uses the common prometheus tooling to build and run some tests.

### Building a Docker container

You can build a Docker container with the included `docker` make target:

    make promu
    promu crossbuild -p linux/amd64 -p linux/arm64
    make docker

This will not even require Go tooling on the host.

### Running in a container

Because chrony only listens on the host localhost, you need to adjust the default chrony address

    docker run \
      -d --rm \
      --name chrony-exporter \
      -p 9123:9123 \
      quay.io/superq/chorny-exporter \
      --chrony-address=host.docker.internal:323

## Running

A minimal invocation looks like this:

    ./chrony_exporter

Supported parameters include:

- `--web.listen-address`: the address/port to listen on (default: `":9123"`)
- `--chrony.address`: the address/port (UDP) or path to Unix socket used to connect to chrony (default: `"[::1]:323"`)
- `--collector.sources`: Enable/disable the collection of `chronyc sources` metrics. (Default: Disabled)
- `--collector.tracking`: Enable/disable the collection of `chronyc tracking` metrics. (Default: Enabled)

To disable a collector, use `--no-`. (i.e. `--no-collector.tracking`)

By default, the exporter will bind on `:9123`.

In case chrony is configured to not accept command messages via UDP (`cmdport 0`) the exporter can use the unix command socket opened by chrony.
In this case use the command line option `--chrony.address=unix:///path/to/chronyd.sock` to configure the path to the chrony command socket.
On most systems chrony will be listenting on `unix:///run/chrony/chronyd.sock`. For this to work the exporter needs to run as root or the same user as chrony.

## Prometheus Rules

You can use [Prometheus rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) to pre-compute some values.

For example, the maximum clock error can be computed from several metrics as [documented in the Chrony man pages](https://chrony.tuxfamily.org/doc/4.3/chronyc.html).

```yaml
groups:
  - name: Chrony
    rules:
      - record: instance:chrony_clock_error_seconds:abs
        expr: >
          abs(chrony_tracking_last_offset_seconds)
          +
          chrony_tracking_root_dispersion_seconds
          +
          (0.5 * chrony_tracking_root_delay_seconds)
```

## TLS and basic authentication

The Chrony Exporter supports TLS and basic authentication.

To use TLS and/or basic authentication, you need to pass a configuration file
using the `--web.config.file` parameter. The format of the file is described
[in the exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).
