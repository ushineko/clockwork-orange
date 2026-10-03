## Validation Report: Spec 022, DuckDuckGo requests with browser-consistent headers

**Date**: 2026-10-03
**Spec**: `specs/022-ddg-browser-consistent-requests.md` (#32)
**Status**: PASSED

### Summary

The landing and `i.js` requests now carry the headers of the Chrome named in
the User-Agent: a navigation, then an XHR. The client hints are built from the
same version constant as the User-Agent. Image downloads are unchanged.

### Phase 3: Tests

- `make test` (`go test -race -tags parity ./...`): all packages ok.
- `TestDuckDuckGoRequestsCarryTheirBrowsersHeaders`: through a fake server and
  a full `Run`, checks every R1 header on `i.js`, every R2 header on the landing
  page, and the absence of `Sec-Fetch-*`, `X-Requested-With` and `Sec-Ch-Ua` on
  the image download.
- `TestDuckDuckGoClientHintsMatchTheUserAgent`: parses the Chrome major version
  from the User-Agent and from both brands in `Sec-Ch-Ua`, and requires them to
  match.
- `TestLiveDuckDuckGoQueriesBackToBackAreNotRefused` (`CLOCKWORK_LIVE_NET=1`):
  12 queries back to back on one client against real DuckDuckGo. Passes with
  this change. **Against the pre-change headers it fails at the 7th query**
  (`scottish fold cats`, HTTP 403), which is the field symptom. This is the
  integration-boundary test.
- E2E: a CLI run with the desk's real config and 7 queries. Every query found
  candidates and none got a 403. `scottish fold cats` saved its first images in
  at least 14 days.
- The tests check what goes over the wire, not how the maps are built.

### Phase 4: Code Quality

- `make lint`: **0 issues**.
- `sessionHeaders` stays as the base, and `landingHeaders`/`resultsHeaders`
  extend it. The client-hint values appear once each per request kind, and
  `ddgSecChUa` is shared. No dead code: the call sites that used
  `sessionHeaders(nil)` and the `Referer`-only map were replaced.

### Phase 5: Security Review

- `govulncheck ./...`: **no vulnerabilities found**.
- No new dependency. No credential handling. The new headers are constants,
  so no user input reaches them; the query still goes only into URL
  parameters through `url.Values`.
- These headers deliberately present the client as the browser its
  User-Agent already named. The Python 2.9.x Windows and macOS builds did the
  same through `ddgs`/`primp`.

### Phase 5.5: Release Safety

- **Rollback**: revert the commit, or reinstall v4.4.1. No on-disk change.

### Not verified

- The first nightly service run after install (E2E step 3) is still pending.
- Windows and macOS were not run by hand. The code has no platform branches,
  and `Sec-Ch-Ua-Platform` says Linux on every OS to match the User-Agent, as
  the User-Agent already did.

### Overall

**PASSED.**
