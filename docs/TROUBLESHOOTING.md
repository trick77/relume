# Troubleshooting

For the common operational issues (cloud suppression, avahi, rootless port 80, entertainment
reconnect) see the **Troubleshooting** section in the [README](../README.md). This document
covers the harder, developer-facing problem: getting the TV to discover relume in the first
place.

## Discovery: the hard part

The single biggest blocker is **coexistence with a powered-on Bridge Pro**. The real Pro also
announces `_hue._tcp` (as `Hue Bridge - XXXXXX` / `modelid=BSB003`), and the TV appears to
de-duplicate and prefer it. Measured: power the Pro **off** and the TV instantly lists relume
and sends `POST /api`; power it on and relume is filtered out. Winning over a powered-on Pro is
an open problem. (relume proxies control *to* the Pro, so testing with the Pro off only
validates discovery and pairing — not light control.)

What the current Philips Android TV actually does during an Ambilight+Hue search (measured):

- It does **not** send a Hue-specific SSDP M-SEARCH and does **not** query
  `discovery.meethue.com`.
- After a TV reboot it actively queries `_hue._tcp.local` and fetches plain `/description.xml`
  through the Android/Dalvik stack, then later sends a `MediaServer:1` SSDP M-SEARCH and fetches
  `/description.xml?relume=ms1` through the Philips DLNA stack.

So **mDNS announce is the primary path.** The working reference is hass-emulated-hue: the mDNS
instance name must be exactly `Philips Hue - XXXXXX` (last 6 of the bridgeid, spaces around the
dash) with TXT bridgeid + modelid. diyHue's `DIYHue-XXXXXX` name is not found by the TV. SSDP is
still served (rootdevice, uuid, basic) but is secondary.

The mDNS announcer must **register exactly once** and never re-announce via a library `Shutdown`
that multicasts an mDNS goodbye (TTL 0) — that evicts relume from the TV's cache and it flickers
out of the Ambilight list.

## Capturing a discovery session

Run a short announcement burst on the Linux/NAS host while the TV is inside its Ambilight+Hue
bridge search, with a packet capture alongside:

```bash
relume serve -debug -advertise-ip <nas-lan-ip> -tv-ip <tv-ip> \
  -discovery-burst-duration 90s -discovery-burst-interval 1s

sudo tcpdump -ni <iface> 'host <tv-ip> or udp port 5353 or udp port 1900 or tcp port 80'
```

Expected signals:
- **Passive mDNS:** relume logs `mdns: burst re-announced as hue bridge`; the TV may then connect
  to `/description.xml` or `/api` without first sending a query.
- **Active mDNS:** relume logs `mdns: query` from `-tv-ip` (even for non-Hue question names).
- **SSDP:** relume logs the TV M-SEARCH and responds immediately; tcpdump shows a follow-up
  `GET /description.xml`.

## Experimental discovery flags

These are debugging knobs for the unsolved coexistence/discovery problem, not user features. They
change relume's wire identity or descriptor behaviour so a single capture can test a hypothesis.

| Flag | Effect |
|------|--------|
| `-identity-profile ambilight` \| `hass` | Switch the SSDP `SERVER` header and `description.xml` manufacturer fields to the Ambilight-OSS or Home Assistant emulated-hue shape. |
| `-description-profile ambilight-reference` | Keep the same identity but match `description.xml` formatting / friendlyName to the Ambilight OSS bridge more closely. |
| `-ssdp-media-server-alias` | Actively broadcast a `MediaServer:1` SSDP NOTIFY and answer `MediaServer:1` M-SEARCH with a cache-busted `LOCATION: /description.xml?relume=ms1`. Only that URL serves `deviceType=MediaServer:1`. |
| `-ssdp-media-server-basic-body` | Keep the `?relume=ms1` MediaServer alias URL the TV follows, but serve a Hue Basic descriptor body from it (tests whether the TV rejects the MediaServer descriptor *type*). |
| `-ssdp-descriptor-variants` | Add an extra query-scoped `/description.xml?relume=basic1` SSDP location for one-scan descriptor-body experiments (use with `-ssdp-media-server-alias`). |

Example combinations:

```bash
# Home Assistant emulated-hue identity:
relume serve -debug -advertise-ip <nas-lan-ip> -tv-ip <tv-ip> \
  -discovery-burst-duration 90s -discovery-burst-interval 1s -identity-profile hass

# If the TV only emits MediaServer:1 SSDP traffic, also try the SSDP alias:
relume serve -debug -advertise-ip <nas-lan-ip> -tv-ip <tv-ip> \
  -discovery-burst-duration 90s -discovery-burst-interval 1s \
  -identity-profile hass -ssdp-media-server-alias

# Match the OSS descriptor body and keep the MediaServer trigger:
relume serve -debug -advertise-ip <nas-lan-ip> -tv-ip <tv-ip> \
  -identity-profile ambilight -description-profile ambilight-reference \
  -ssdp-media-server-alias -ssdp-media-server-basic-body
```

## Experiment history

Each row is a hypothesis tested against the real TV. None has reached `POST /api` with a
powered-on Pro yet.

| Version | Variation | Result |
|---------|-----------|--------|
| `0.1.8` | Ambilight identity profile, OSS-emulator headers, short CLIP v1 config + compatibility endpoints. | TV stopped after descriptor discovery. |
| `0.1.9` | HTTP `Server`/`Cache-Control` on `description.xml`; MediaServer alias `max-age=1`. | No `/api` follow-up. |
| `0.1.10` | mDNS SRV host changed to lower bridgeid (`<bridgeid>.local.`). | TV HTTP `Host` stayed the IP → hostname multiplexing not useful. |
| `0.1.11` | Ambilight serial, UDN, SSDP UUID/USN changed to lower bridgeid with `FFFE`. | No `/api` follow-up. |
| `0.1.12` | Basic:1 SSDP USN changed to `uuid::<urn:...:basic:1>`. | After reboot the TV fetched plain `/description.xml` and `?relume=ms1`; still no `/api`. |
| `0.1.13` | Added `-ssdp-descriptor-variants` (`?relume=basic1` Basic body). | Windows Chromium/DIAL fetched `basic1`; the TV fetched only plain `/description.xml` and `?relume=ms1`. Still no `/api`. |
| `0.1.15` | Added `-description-profile ambilight-reference`. | TV fetched the changed `?relume=ms1` bytes; still no `/api`. |
| `0.1.16` | Added `-ssdp-media-server-basic-body`. | Basic body from the `?relume=ms1` URL. |
| next | mDNS register-once; removed Shutdown-based re-announce (it emitted goodbye/TTL-0 packets that evicted the bridge from the TV cache). | Root cause of the flicker; the confirmed-working 83noit emulator registers once. |
