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
                                 Address of the Chrony server.
      --chrony.timeout=5s        Timeout on requests to the Chrony server.
      --[no-]collector.tracking  Collect tracking metrics
      --[no-]collector.sources   Collect sources metrics
      --[no-]collector.sourcestats
                                 Collect sourcestats metrics
      --[no-]collector.sources.with-ntpdata  
                                 Extend sources with ntpdata metrics (requires socket connection)
      --[no-]collector.serverstats  
                                 Collect serverstats metrics
      --[no-]collector.clients   Collect clients metrics
      --[no-]collector.chmod-socket  
                                 Chmod 0666 the receiving unix datagram socket
      --[no-]collector.dns-lookups  
                                 do reverse DNS lookups
      --collector.dns-ptr-cache-min-ttl=0s  
                                 minimum time to cache a reverse DNS lookup;
                                 raises short/absent record TTLs to at least
                                 this. 0 (default) honors the record's own TTL
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

### Reverse DNS lookups

`--collector.dns-lookups` resolves source addresses to names using two
resolvers: the system's own (`/etc/hosts`, DNS, mDNS, NIS, LDAP - whatever
`/etc/nsswitch.conf` or the platform equivalent configures), and a direct
DNS query.

The system resolver is the sole authority on the returned name, even when
it disagrees with the direct DNS query. If it fails, the raw address is
returned - the direct DNS query's name is never shown, even if it found
one. Presenting a name from bypassing the system's own resolution path
would treat DNS as authoritative for a host that may not configure it
that way (e.g. `dns` deliberately excluded from `nsswitch.conf`). The
direct DNS query exists only to supply a record TTL, which Go's standard
resolver doesn't expose.

Results are cached for that TTL by default
(`--collector.dns-ptr-cache-min-ttl` raises the floor, see above) - but
only when the two answers agree: an "agree" doesn't need an exact match,
since PTR has no alias concept and a system answer may legitimately carry
an extra `/etc/hosts`-style alias DNS doesn't know about; it just needs
every name DNS reported to also be one the system reported. On a genuine
disagreement, or when the system resolver fails outright, the cache falls
back to the configured floor instead (the DNS TTL is still used to pace
retries in the fails-outright case, just doesn't make the name eligible to
be returned). Both resolvers run at most once per cache entry, not per
scrape.

An address with no PTR record at all (common for public NTP pool/anycast
servers) is cached too: an authoritative NXDOMAIN or NODATA answer's own
[RFC 2308](https://www.rfc-editor.org/rfc/rfc2308) negative cache TTL
(from its SOA record) is honored the same way a positive TTL is.

Two situations leave no usable TTL - re-queried at the floor every scrape
- and each logs a one-time `WARN` the first time it's seen, for the life
of the process (not gated by the floor, since a real TTL is exactly
what's missing):

* The system resolver and direct DNS query keep disagreeing.
* Neither resolver produces a name, and DNS gave no usable TTL either.

### Clients collector

`--collector.clients` summarizes chronyd's client log, the same data shown by
`chronyc clients`, into a few aggregate metrics with no per-client labels, so
the number of series stays the same no matter how many clients connect:

* `chrony_clients_connected{protocol}` counts the clients in the log, split
  into `nts` (clients that completed an NTS-KE handshake) and `ntp` (the rest).
* `chrony_clients_last_ntp_hit_ago_seconds` and `chrony_clients_ntp_interval_seconds`
  are histograms of how recently each client was seen and how often it polls.
* `chrony_clients_ntp_drops` is a histogram of how many NTP requests were
  dropped (rate limited) per client.

This needs chrony 4.0 or later. The command it uses reports NTS-KE statistics,
which chronyd only gained with NTS support in 4.0. Older versions do not know
the command and reject the request, so the scrape sets `chrony_up` to 0.

The client log size is set by
[`clientloglimit`](https://chrony-project.org/doc/4.5/chrony.conf.html#clientloglimit)
in `chrony.conf`. This is a limit in bytes, 512 KiB by default, which chronyd
uses to hold a power-of-two number of records, around 4096 with the default.
When the log is full chronyd reuses the oldest records, so the connected count
stops at that limit. Increase `clientloglimit` to track more clients, but note
that it is real memory (up to 2 GB), so set it to match the number of clients
you expect.

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
