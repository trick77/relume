# relume — Architecture Review (remaining items)

Original review date: 2026-06-17 · Scope: full `internal/*` + `cmd/relume` + build/CI.
Security was out of scope by request (LAN-only, no external access).

> **Status:** 13 of the original findings were remediated in #67 and verified against the
> code (H1–H3, M1, M2, M3, M6, L1, L3, L4, L8, L9, L10) — they have been removed from this
> document. What remains below is the open tech-debt backlog: **M4, M5, M7, L2, L5, L6** and
> the partial **L7**.

## Verdict (unchanged)

A well-structured small Go codebase. Clean layering (no import cycles, `config` is a proper
leaf, `clipv1` uses textbook dependency inversion), hard-won protocol knowledge preserved as
deliberate invariants, and the trickiest mechanisms — the coalescing optimistic provider, the
REST/DTLS mutual exclusion, the sticky fallback watchdog — are sound and well-commented.

The remaining weaknesses cluster in two places: (1) two large "god" units (`runServe`, the
triplicated pairing sequence) with no test seam around the orchestration that matters most,
and (2) a handful of defensive nits and conscious-decision items.

---

## Open findings

### M4 — `runServe` is a ~250-line orchestration with no test seam  ·  *Medium · Medium effort*
`cmd/relume/main.go` does flag validation, IP detection, mode selection,
provider/SSDP/mDNS/HTTP construction, 5+ goroutine launches, the entertainment/rest branch,
and shutdown/flash inline. Only the extracted pure helpers are tested; the wiring, ordering
and shutdown have zero coverage. The most failure-prone code (`autoPairPro`/`watchPro`
re-discovery, re-pin, hot-swap) is untested because `*bridgepro.Client` is taken concretely
everywhere except the `proClient` interface in `provider.go` — there's no seam to inject a
fake. **Fix:** extract an `application` struct (build deps → `Run(ctx)`); define the
Pro-client interface in `bridgepro` so the resilience helpers are unit-testable.

### M5 — Duplicated Pro-pairing sequence (×3)  ·  *Medium · Medium effort*
Discover → `FetchLeafFingerprint` → `HTTPClientFor` → `Pair` → `BridgeInfo` → `SetPro` →
list lights is implemented nearly verbatim in `runSetup` and `autoPairPro`, with the
reconnect path repeating the discover+fingerprint half. This is the most
correctness-sensitive code (cert pinning, key persistence) living in three copies in the
entrypoint. **Fix:** extract a `Pairer`/`Reconnector` into `bridgepro`.

### M7 — `handleGroupAction` silently drops Ambilight frames  ·  *Medium · Medium effort*
`PUT /groups/{id}/action` is logged and acked but never forwarded. If a TV/firmware ever
drives lights via the group path, lights silently won't follow while relume reports success —
the `recordGroupActionWrite` tally exists precisely to detect this. Currently documented as
safe for the target TV (per-light PUT only). **Fix:** forward through the same provider, or
keep the documented justification and the tally as the tripwire.

### L2 — No bounded queue between DTLS decode and forward  ·  *Low*
`entertainment/receiver.go`: `OnFrame`/`Push` run synchronously on the single reader
goroutine. Safe only because every callback is currently non-blocking; a future slow
`fallback` would stall intake with silent UDP drop and no backpressure. Document as a hard
invariant or add a select/drop stage.

### L5 — Config has no schema version / no fsync / orphaned `.tmp` on rename failure  ·  *Low*
`config.go`. Additive evolution works today; a rename or restructure has no migration path,
and a garbage file fails hard with no quarantine. Add a `schemaVersion` field now.

### L6 — `health check = Lights()`  ·  *Low*
A heavy call used as a liveness probe; `BridgeInfo` is the lighter ping already available.

### L7 — Unguarded slices / narrow type handling  ·  *Low · partial*
`lights.go` (`l.ID[:8]` panics on short id); `control.go` `toFloat` omits `int64` (same bug
class as the "stuck red" bug, reachable via the in-process entertainment `bri` path); v2
channel id truncated to 8 bits without a `<=255` guard (`huestream.go`). All low-risk
defensive nits; not yet addressed.

---

## Remaining test gaps

| Area | Tested? |
|---|---|
| `bridgepro/client.go` (get/post/put/del, **207 multi-status**, cert pin), `resources.go`, `discover.go` | **No** |
| `cmd/relume` orchestration: `autoPairPro`/`watchPro`/re-pin/hot-swap | **No** (no seam — see M4) |

## Suggested sequence for the remainder

1. **M5 + M4 together** — a `Pairer` in `bridgepro` and an `application` seam unlock testing
   the resilience path and remove the triplication.
2. **L2 / L5 / L6 / L7** — opportunistic defensive cleanups.
3. **M7** — decide forward-vs-document once the target-TV behaviour is fully confirmed.
