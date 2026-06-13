# relume

A software bridge that connects a **Philips Ambilight TV** to a **Hue Bridge Pro (BSB003)**.
relume presents itself to the TV as an old gen-2 bridge (BSB002) and proxies every request
to the real Bridge Pro over HTTPS/CLIP v2.

```
Ambilight TV  ──mDNS/SSDP + HTTP──▶  relume  ──HTTPS/CLIP v2──▶  Hue Bridge Pro  ──Zigbee──▶  lights
```

Background and design: see [PLAN.md](PLAN.md) and [AGENTS.md](AGENTS.md).

## Requirements

- relume must run on the **same L2 network** as the TV (discovery uses multicast).
  → Docker requires `network_mode: host`.
- A reachable Hue Bridge Pro on the same network.

## Quick start (Docker)

```bash
# 1. Pair with the real Bridge Pro (once). When prompted, briefly TAP the link
#    button on the Bridge Pro (do not hold it).
docker compose run --rm relume setup -config /data/relume.json
#    add -bridge-ip <ip> if cloud discovery finds nothing.

# 2. Start the service
docker compose up -d

# 3. On the TV, start the Ambilight+Hue bridge search. When the TV asks for the
#    link button, open the pairing window:
docker compose run --rm relume link        # or in a browser: http://<host-ip>/
```

The image is pulled from `ghcr.io/trick77/relume` (built by the release workflow).
To build locally instead: `docker build -f Containerfile -t relume:dev .`

## Commands

| Command | Purpose |
|---------|---------|
| `serve` | Run the service (discovery + bridge emulation). Default. |
| `setup` | Pair with the Bridge Pro (fetch app key, pin certificate). |
| `discover` | Find the Bridge Pro via Philips cloud. |
| `link` | Open the pairing window (30s) for the TV. |
| `avahi-service` | Emit an Avahi service file (see mDNS caveat). |
| `version` | Print the version. |

Useful `serve` flags: `-http-port` (default 80), `-advertise-ip` (empty = auto),
`-debug` (SSDP/HTTP diagnostics + mDNS observer).

## Important caveats

### Discovery: the TV uses mDNS, not SSDP
Measured against a real Philips TV: the Hue search does **not** use SSDP, it uses
mDNS (`_hue._tcp`). The TV does not actively query — it **passively listens** for the
bridge's announcement. relume actively announces `Philips Hue - XXXXXX` / `modelid=BSB002`.
The real Bridge Pro announces itself as `BSB003`, which the TV rejects as incompatible.

**mDNS conflict with avahi:** if the host runs an `avahi-daemon` (it owns UDP 5353),
relume's built-in mDNS announcer cannot bind the port. In that case let avahi announce:
```bash
docker compose run --rm relume avahi-service -config /data/relume.json > /etc/avahi/services/relume-hue.service
# match the port to the serve http-port: relume avahi-service -http-port 80
```
Alternatively disable `avahi-daemon`, then relume's own announcer works.

### Cloud suppression
If a real Hue bridge is registered at `discovery.meethue.com`, the TV may resolve it via
the cloud and **skip local discovery** (diyHue #988). Check with
`curl https://discovery.meethue.com/` from the TV's network — if it returns the real bridge,
redirect `discovery.meethue.com` to relume via local DNS.

### Rootless Docker and port 80
A real bridge speaks on port 80. Under **rootless** Docker, ports <1024 require a host sysctl:
```bash
sudo sysctl net.ipv4.ip_unprivileged_port_start=80   # do NOT run the container as root
```
Alternatively use a high port (`-http-port 8080`) — works as long as the TV honors the
port advertised via mDNS (to be verified).

## Persistence / secrets

State (bridge identity, TV tokens, **Bridge Pro app key + clientkey**) lives in
`./data/relume.json`. This file holds secrets — do not share or commit it (it is gitignored).

## Build / test (local)

```bash
go build -o relume ./cmd/relume
go test ./...
```
