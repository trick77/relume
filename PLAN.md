# relume — plan & status

Software bridge connecting a **Philips Ambilight TV** to a **Hue Bridge Pro (BSB003)** by
emulating an old gen-2 bridge (BSB002) toward the TV and proxying everything to the real
Bridge Pro over HTTPS/CLIP v2.

```
Ambilight TV  ──mDNS/SSDP + HTTP──▶  relume  ──HTTPS/CLIP v2──▶  Hue Bridge Pro  ──Zigbee──▶  lights
```

## Why

The Bridge Pro breaks the Ambilight+Hue path in three ways:
1. **No SSDP/UPnP** — only mDNS + cloud; the TV firmware expects to discover via the local bridge.
2. **HTTPS:443 only** — no plain HTTP:80 (returns 301); the TV firmware is wired for HTTP.
3. **CLIP v2 only** — the v1 discovery/pairing paths the TV uses no longer resolve.

## Decisions

| Topic | Decision |
|------|----------|
| Base | Standalone Go proxy (diyHue is reference only, not a fork) |
| Language | Go |
| Deployment | Docker with `network_mode: host` (multicast discovery needs the TV's L2) |
| Lights | Proxied live from the Bridge Pro |
| Path | Full: Entertainment + REST fallback |
| Bridge Pro setup | One-time pairing; pin the TLS certificate (default), `-skip-tls-verify` fallback |
| File naming | `Containerfile`, `compose.yaml`; CI workflows `.yaml` |

## Architecture

**Frontend (TV-facing, emulates BSB002):**
- `internal/ssdp` — multicast responder (M-SEARCH) + periodic NOTIFY ssdp:alive.
- `internal/mdns` — active `_hue._tcp` announcer (`Philips Hue - XXXXXX`, TXT bridgeid+modelid=BSB002).
- `internal/upnp` — `/description.xml` with the BSB002 identity.
- `internal/clipv1` — HTTP server: pairing (`POST /api`, link window), `config`, lights/groups, REST control.

**Backend (Bridge Pro-facing, acts as a Hue app):**
- `internal/bridgepro` — CLIP v2 client (HTTPS + cert pinning), pairing, resource reads, REST control. *(Entertainment client: M4)*

**Core:**
- `internal/config` — persistent state: identity, TV tokens, Pro pairing, light mapping.
- `internal/translate` — v1↔v2 translation + v1-id↔UUID mapping.
- `internal/bridge` — wiring frontend↔backend.
- `internal/diag` — passive mDNS observer (debug).
- `cmd/relume` — `serve` (default), `setup`, `discover`, `link`, `avahi-service`, `version`.

## Milestones

- **M1 — Discovery & pairing** ✅ done & verified. TV finds & pairs the bridge.
- **M2 — Bridge Pro client** ✅ done & verified on the real BSB003. CLIP v2 client (HTTPS +
  cert pinning), `setup`/`discover`, v2→v1 light list (16 lights via the proxy).
- **M3 — REST control** ✅ light control done & verified (real lamp switched). `PUT lights/{id}/state`
  → v1→v2 → Pro (207/errors handled). Group path is still a logged stub (completed with M4).
- **mDNS discovery** ✅ implemented (active `_hue._tcp` announce) + `avahi-service` command.
  Final TV-detection test pending on Linux (see below).
- **M4 — Entertainment** ⏳ open. `huestream` (+tests), DTLS server (TV) + DTLS client (Pro),
  entertainment-config activation, stream forwarding. Goal: smooth Ambilight.
- **M5 — Packaging** ✅ done. Containerfile (static, multi-stage), `compose.yaml` (host networking),
  README, CI (test + release to ghcr.io). Image builds.

## Discovery finding (measured on the real Philips TV)

- The TV (a 2021/22 Philips Android TV) sends SSDP M-SEARCH **only** for `MediaServer` (DLNA) —
  never anything Hue-related, never fetches `/description.xml`. **→ no SSDP for Hue.**
- **No cloud:** no DNS lookup for `discovery.meethue.com` (checked). Cloud discovery ruled out.
- **No active `_hue._tcp` query.** Conclusion (consistent with diyHue): the TV **listens passively**
  for the bridge's `_hue._tcp` mDNS announcement. relume announces exactly that.
- The real Bridge Pro itself announces `_hue._tcp` as `Hue Bridge - XXXXXX` / `modelid=BSB003`;
  the TV likely filters BSB003 out. relume announces `Philips Hue - XXXXXX` / `modelid=BSB002`.
- UDP 10102 broadcasts from the TV are DTS Play-Fi (audio) — a red herring, unrelated to Hue.
- macOS is an unusable test environment: the system mDNSResponder owns port 5353, so relume's
  built-in announcer cannot bind it. Final TV test belongs on single-homed Linux (the NAS).

## Open items (verify on the real device)

- **TV detection of relume via mDNS on the Linux target** (the decisive open test).
- Exact `HueStream` v2 layout (52-byte header, channel chunks).
- Exact CLIP v2 calls to create/activate the `entertainment_configuration` on the Pro.
- Whether the TV requires a specific `swversion`/`apiversion` to attempt Entertainment.
- The exact `devicetype` string the TV sends to `POST /api`; whether the TV uses the
  mDNS-advertised port or hardcodes 80.

## Build / test

```bash
go build -o relume ./cmd/relume
go test ./...
go run ./cmd/relume serve -debug -http-port 8080 -advertise-ip <ip> -config ./relume.json
```
