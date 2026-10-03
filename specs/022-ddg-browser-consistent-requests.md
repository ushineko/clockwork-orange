# Spec 022: DuckDuckGo requests that look like the browser they claim to be

**Issue**: #32

## Status: COMPLETE

- **Priority**: High
- **Estimated Complexity**: Low
- **Branch**: `fix/ddg-client`

## Executive Summary

The DuckDuckGo plugin sends a Chrome User-Agent and none of the other headers
Chrome sends. After a few queries DuckDuckGo answers `i.js` with HTTP 403, and
the rest of the run gets no candidates. The `i.js` request now carries the
header set a browser sends on that XHR call, and the landing request carries a
navigation's headers. Measured under an active block, the new request passed
every time the old one failed.

## Context

### The regression

2.9.x had two paths (spec 008). Windows and macOS used the `ddgs` library, whose
HTTP client (`primp`) impersonates a real browser: TLS, HTTP/2 and headers.
Linux used a direct scrape that sent only a User-Agent and `Accept-Language`
to the landing page, plus a `Referer` to `i.js`. Spec 010 (D9/DV9) kept only the
direct path, so v4 sends the Linux 2.9.x request on every platform.

Seen in the field (Linux, v4.4.0, last 14 days of nightly runs): the 7th of 7
queries returned `i.js returned HTTP 403` on every run, so it never produced a
candidate.

### Measurements (2026-10-03, one IP, Go's TLS stack throughout)

| Experiment | Result |
|---|---|
| curl, any headers | 403 on `i.js` every time (TLS signature) |
| Go, current headers, back to back | 3–6 queries pass, then 403 on every later one |
| Block active; current and new clients alternating 0.6 s apart | current: 5 of 5 got 403; new: 6 of 6 got 200 |
| New client, 24 queries back to back | 24 of 24 got 200; the current client passed straight after |
| Ablation under a block, 3 rounds each | full `i.js` set: 3 of 3 passed; landing headers only, `Sec-Fetch-*` only, `Accept` only, `ct=AT` only: 0–1 of 3 |

The landing page answers 200 with a valid `vqd` throughout a block, so the IP
is not refused. A 403 body is only `If this error persists, please let us
know: ops@duckduckgo.com`, with no `Retry-After` and no rate-limit headers.

### Conclusion

DuckDuckGo blocks the request signature, not the IP: it reacts to the volume
of requests that do not look like the browser in the User-Agent. Request
headers alone are enough to look like that browser. The TLS signature is not
the trigger, because both clients used Go's TLS stack.

## Requirements

- **R1**: The `i.js` request carries `Accept`, `Accept-Language`, `Referer`,
  `Sec-Fetch-Dest: empty`, `Sec-Fetch-Mode: cors`, `Sec-Fetch-Site:
  same-origin`, `X-Requested-With: XMLHttpRequest`, and `Sec-Ch-Ua`,
  `Sec-Ch-Ua-Mobile` and `Sec-Ch-Ua-Platform`.
- **R2**: The landing request carries a top-level navigation's headers:
  `Accept`, `Accept-Language`, `Sec-Fetch-Dest: document`, `Sec-Fetch-Mode:
  navigate`, `Sec-Fetch-Site: none`, `Sec-Fetch-User`,
  `Upgrade-Insecure-Requests`, and the three `Sec-Ch-Ua*` headers.
- **R3**: `Sec-Ch-Ua` names the same Chrome major version as the User-Agent,
  and `Sec-Ch-Ua-Platform` the same platform. They are built from one
  constant, so updating the User-Agent cannot leave them disagreeing.
- **R4**: Image downloads are unchanged: User-Agent, `Accept-Language`, and the
  source page as `Referer`.
- **R5**: Query parameters, filtering, history and download behaviour are
  unchanged.

## Acceptance Criteria

- [x] `i.js` request carries every R1 header, with the values a browser sends
      (fake-server test).
- [x] Landing request carries every R2 header (fake-server test).
- [x] `Sec-Ch-Ua`'s Chrome major version equals the User-Agent's (unit test
      parsing both).
- [x] Image downloads send no `Sec-Fetch-*` or `X-Requested-With` header (R4).
- [x] Live, against DuckDuckGo: 12 different queries back to back through the
      plugin's own scrape path all return candidates, with no 403
      (`CLOCKWORK_LIVE_NET=1`).
- [x] `make test` passes, `make lint` reports 0 issues, `govulncheck` is clean.

## Risks & Assumptions

- **DuckDuckGo can change its checks.** This holds for the signature measured
  on 2026-10-03. If DuckDuckGo starts checking TLS or HTTP/2 signatures, a 403
  will return, and the fallback is a browser-impersonating transport (`utls`
  plus HTTP/2 settings). The live test is what will notice.
- **The User-Agent stays at Chrome 132.** R3 keeps the hints in step with it.
  Moving to a newer version is a one-constant change.
- **Rollback**: revert the commit. No on-disk change.

## E2E Test Plan

| Step | Environment | Expected | Covers |
|---|---|---|---|
| `CLOCKWORK_LIVE_NET=1 go test -run TestLiveDuckDuckGo ./internal/plugins/` | Linux desk | all live DuckDuckGo tests pass | AC5 |
| Install the build; force a run: `clockwork-orange plugin run duckduckgo_images --force` (or Download now in the GUI) with the desk's 7 queries | Linux desk | every query logs `Found N potential images`, none `i.js returned HTTP 403` | AC5, R1 |
| Read the next nightly run in `journalctl --user -u clockwork-orange` | Linux desk | no `HTTP 403` lines | R1 |

## Alternatives Considered

- Considered spacing queries and retrying with growing waits on a 403;
  rejected as the fix. Under a block, spacing did not help the current client,
  and the new client needed no spacing for 24 queries. Pacing remains a
  candidate for the separate "fetch later result pages" work.
- Considered `utls` with Chrome's HTTP/2 settings; rejected for now. It adds a
  network dependency and is not needed: the headers alone pass on Go's TLS.

## E2E Results (2026-10-03)

- **Step 1:** `CLOCKWORK_LIVE_NET=1 go test -run TestLiveDuckDuckGo ./internal/plugins/`
  passes. The same burst test run against the pre-change headers fails at the
  7th query: `"scottish fold cats": i.js returned HTTP 403`, which is the
  field symptom.
- **Step 2:** `clockwork-orange plugin run duckduckgo_images --force
  --plugin-config '{"limit":3}'` with the desk's 7 queries. Every query logged
  `Found N potential images` (21–60), with no 403. `scottish fold cats` found
  23 and saved 3, its first downloads in at least 14 days.
- **Step 3:** pending. Read the first nightly run after the release is
  installed.
