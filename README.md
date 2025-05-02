# Prometheus Chrony Exporter

[![Build Status](https://circleci.com/gh/SuperQ/chrony_exporter/tree/main.svg?style=svg)](https://circleci.com/gh/SuperQ/chrony_exporter/tree/main)
[![Docker Repository on Quay](https://quay.io/repository/superq/chrony-exporter/status "Docker Repository on Quay")](https://quay.io/repository/superq/chrony-exporter)
[![Go Reference](https://pkg.go.dev/badge/github.com/superq/chrony_exporter.svg)](https://pkg.go.dev/github.com/superq/chrony_exporter)

This is a [Prometheus Exporter](https://prometheus.io) for [Chrony NTP](https://chrony-project.org/).

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
      quay.io/superq/chrony-exporter \
      --chrony.address=host.docker.internal:323

## Running

A minimal invocation looks like this:

    ./chrony_exporter

Supported parameters include:

```
usage: chrony_exporter [<flags>]


Flags:
  -h, --[no-]help                Show context-sensitive help (also try --help-long and --help-man).
      --chrony.address="[::1]:323"  
                                 Address of the Chrony srever.
      --chrony.timeout=5s        Timeout on requests to the Chrony srever.
      --[no-]collector.tracking  Collect tracking metrics
      --[no-]collector.sources   Collect sources metrics
      --[no-]collector.sources.with-ntpdata  
                                 Extend sources with ntpdata metrics (requires socket connection)
      --[no-]collector.serverstats  
                                 Collect serverstats metrics
      --[no-]collector.chmod-socket  
                                 Chmod 0666 the receiving unix datagram socket
      --[no-]collector.dns-lookups  
                                 do reverse DNS lookups
      --web.telemetry-path="/metrics"  
                                 Path under which to expose metrics.
      --[no-]web.systemd-socket  Use systemd socket activation listeners instead of port listeners (Linux only).
      --web.listen-address=:9123 ...  
                                 Addresses on which to expose metrics and web interface. Repeatable for multiple
                                 addresses.
      --web.config.file=""       Path to configuration file that can enable TLS or authentication. See:
                                 https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md
      --log.level=info           Only log messages with the given severity or above. One of: [debug, info, warn,
                                 error]
      --log.format=logfmt        Output format of log messages. One of: [logfmt, json]
      --[no-]version             Show application version.
```

To disable a collector, use `--no-`. (i.e. `--no-collector.tracking`)

By default, the exporter will bind on `:9123`.

In case chrony is configured to not accept command messages via UDP (`cmdport 0`) the exporter can use the unix command socket opened by chrony.
In this case use the command line option `--chrony.address=unix:///path/to/chronyd.sock` to configure the path to the chrony command socket.
On most systems chrony will be listenting on `unix:///run/chrony/chronyd.sock`. For this to work the exporter needs to run as root or the same user as chrony.
When the exporter is run as root the flag `collector.chmod-socket` is needed as well.

## Prometheus Rules

You can use [Prometheus rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) to pre-compute some values.

For example, an absolute bound on the clock accuracy can be computed from several metrics as [documented in the Chrony man pages](https://chrony-project.org/doc/4.6.1/chronyc.html).

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
