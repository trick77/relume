# AGENTS.md — relume

Module `github.com/trick77/relume`. Binary `relume`. Dir still named `ambibridge` (cosmetic).
Emulates a gen-2 Hue Bridge (BSB002) toward a Philips Ambilight TV; proxies to a real Hue
Bridge Pro (BSB003) via CLIP v2.

All repo content (docs, code comments, logs) is English.

## build/test
- `go build -o relume ./cmd/relume`
- `go test ./...`
- diagnostics: `relume serve -debug` (SSDP header log + mDNS observer + HTTP body log)
- commands: `serve` (default), `setup` (pair Pro), `discover` (cloud), `link` (open 30s TV pairing window), `avahi-service`, `version`
- container build file is `Containerfile` (not Dockerfile); compose file is `compose.yaml`

## identity invariants (TV rejects otherwise)
- `modelid` MUST be `BSB002` everywhere: mDNS TXT, description.xml, /config.
- bridgeid = upper(serial[:6] + "FFFE" + serial[6:]); serial = 12 hex; UUID = `2f402f80-da50-11e1-9b23-<serial>`.
- UUID identical across SSDP USN, description.xml UDN. bridgeid identical across SSDP hue-bridgeid header, mDNS TXT, /config.

## discovery (the hard part)
- Measured: the TV does NOT send hue SSDP M-SEARCH (only `MediaServer`), does NOT use cloud (no DNS for discovery.meethue.com), does NOT actively query `_hue._tcp`. It LISTENS passively for the `_hue._tcp` mDNS announcement.
- So mDNS announce is the primary path. Working ref = hass-emulated-hue: instance name exactly `Philips Hue - XXXXXX` (last 6 of bridgeid, spaces around dash), TXT bridgeid+modelid. diyHue name `DIYHue-XXXXXX` NOT found by TV.
- The real Bridge Pro also announces `_hue._tcp` as `Hue Bridge - XXXXXX` / `modelid=BSB003`. TV likely filters BSB003 out.
- Port 10102 broadcasts from the TV are DTS Play-Fi (audio), a red herring — not Hue.
- SSDP still served (3 ST: rootdevice, uuid, basic) but secondary. Respond instantly (short TV search window).
- multi-NIC: bind multicast to the interface owning advertise-IP, else Go uses the default iface (wrong LAN). Dual-homed host = bad test env. macOS system mDNSResponder owns 5353 → built-in announcer fails there; test on Linux (NAS).

## Bridge Pro (BSB003) facts
- HTTPS:443 only; HTTP:80 → 301. CLIP v2 only.
- cert self-signed Signify (CN=root-bridge, leaf OU=BSB003) → pin leaf SHA-256, do NOT trust CA chain. `-skip-tls-verify` fallback.
- pair = POST https://<ip>/api {devicetype,generateclientkey:true}; physical button = brief TAP not hold; error 101 = not pressed.
- PUT returns 207 multi-status with per-attribute `errors[]` even when HTTP-ok → inspect errors[], not just status code.
- CT-only lights reject `color.xy` → 207 error. v2 lights have no reliable id_v1 → assign stable v1 ids by sorted-UUID order.

## deployment
- needs same L2 as TV (SSDP+mDNS multicast) → Docker `network_mode: host`.
- rootless can't bind <1024. If TV hardcodes API port 80 (unconfirmed; SRV/LOCATION port may be honored instead), use host `sysctl net.ipv4.ip_unprivileged_port_start=80`, NOT a root container.
- CI: push/PR to master runs tests; push to master builds+pushes image to ghcr.io/trick77/relume (semver tag auto-bumped).

## toolchain trap
- go 1.26 + grandcat/zeroconf v1.0.0 pulls ancient golang.org/x/net that fails to link (`syscall.recvmsg`). Keep x/net, x/sys, x/crypto upgraded.

## secrets
- `relume.json` holds Pro appKey/clientkey + TV tokens. Gitignored. Never commit.

## status
M1 discovery/pairing, M2 Pro client, M3 REST light control: done+verified on real Pro. M4 entertainment (DTLS+HueStream) not started. Final TV discovery test pending on Linux. See PLAN.md.
