#!/usr/bin/env python3
"""DuckDuckGo transport diagnostic probe.

Purpose: replace the *unverified* "TLS-fingerprint soft-block" hypothesis in
validation-reports/2026-04-17-1646-ddg-ddgs-backend.md with actual measured
data. Run this from every environment we ship or build in (Linux system
Python, macOS framework build, frozen Windows/macOS exe) and diff the output.

It answers three concrete questions, none of which the original report
measured:

  1. What OpenSSL/TLS stack is this build actually using?
  2. Does the TLS handshake fingerprint (JA3/JA4) differ between `requests`
     (system/OpenSSL) and `primp` (browser-impersonating), and does either
     match a real browser?
  3. Does the real DuckDuckGo `i.js` call get a full body or the tell-tale
     "200 + empty body" soft-block — via requests vs via primp?

If requests gets blocked and primp does not, and their fingerprints differ,
the TLS-block diagnosis is confirmed and primp is justified. If the failure
reproduces regardless of transport, or the fingerprints are identical, the
root cause is something else (CA bundle, ALPN/HTTP2, trust store) and primp
is the wrong fix.

Standalone: `python3 ddg_probe.py`
In a frozen build: `clockwork-orange --ddg-probe`
"""

import json
import platform
import re
import ssl
import sys

# Same query + params the plugin uses, so the probe reflects real behaviour.
# Keep these in sync with plugins/duckduckgo_images.py._scrape_via_direct.
_QUERY = "4k nature wallpapers"
_IJS_PARAMS = {
    "l": "us-en",
    "o": "json",
    "q": _QUERY,
    "f": "type:photo,size:Large,layout:Wide",
    "p": "1",
}
_UA = (
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.36"
)
# tls.peet.ws echoes the JA3/JA4 it saw; browserleaks is a fallback.
_FP_PRIMARY = "https://tls.peet.ws/api/all"
_FP_FALLBACK = "https://tls.browserleaks.com/json"


def _hr(title):
    print("\n" + "=" * 4 + " " + title + " " + "=" * (60 - len(title)))


def _mod_version(name):
    try:
        m = __import__(name)
        return getattr(m, "__version__", "(no __version__)")
    except Exception as e:  # noqa: BLE001 - probe must never crash
        return f"(unavailable: {e})"


def _environment():
    _hr("ENVIRONMENT")
    print(f"python           : {sys.version.split()[0]}")
    print(f"platform         : {sys.platform} / {platform.platform()}")
    print(f"frozen           : {getattr(sys, 'frozen', False)}")
    meipass = getattr(sys, "_MEIPASS", None)
    if meipass:
        print(f"bundle (_MEIPASS): {meipass}")
    print(f"OpenSSL          : {ssl.OPENSSL_VERSION}")
    try:
        import certifi

        print(f"certifi          : {certifi.__version__} @ {certifi.where()}")
    except Exception as e:  # noqa: BLE001
        print(f"certifi          : (unavailable: {e})")
    print(f"requests         : {_mod_version('requests')}")
    print(f"primp            : {_mod_version('primp')}")
    print(f"ddgs             : {_mod_version('ddgs')}")


# --- transports -----------------------------------------------------------
# Each transport exposes get(url, params, headers) -> (status, text) so the
# fingerprint check and the DDG flow can run identically through both.


def _requests_transport():
    import requests

    session = requests.Session()
    session.headers.update({"User-Agent": _UA})

    def get(url, params=None, headers=None):
        r = session.get(url, params=params, headers=headers, timeout=20)
        return r.status_code, r.text

    return get


def _primp_default_transport():
    """primp configured exactly the way ddgs 9.x drives it: no impersonation.

    This is the representative "what the shipped app's transport does" client.
    cookie_store=True so the vqd session cookie from the landing page is
    carried into the i.js call (without it DDG returns 403 regardless).
    """
    import primp

    try:
        client = primp.Client(timeout=20, cookie_store=True)
    except TypeError:
        client = primp.Client()

    def get(url, params=None, headers=None):
        r = client.get(url, params=params, headers=headers)
        return r.status_code, r.text

    return get


def _primp_impersonate_transport(impersonate="chrome_124", os_name="windows"):
    """primp with a pinned desktop-browser TLS profile.

    Included to test the "impersonate a real browser" hypothesis directly.
    impersonate_os is pinned because primp randomizes the OS per client
    otherwise (yielding mobile UAs that skew the result). If the token is
    unknown to this primp version it silently falls back to 'random' — the
    UA-as-seen line in the fingerprint section shows what was actually used.
    """
    import primp

    kwargs = dict(timeout=20, cookie_store=True, impersonate=impersonate)
    try:
        client = primp.Client(impersonate_os=os_name, **kwargs)
    except TypeError:
        client = primp.Client(**kwargs)

    def get(url, params=None, headers=None):
        r = client.get(url, params=params, headers=headers)
        return r.status_code, r.text

    return get, impersonate


def _fingerprint(label, get):
    """Fetch a TLS echo endpoint and surface JA3/JA4 for this transport."""
    for url in (_FP_PRIMARY, _FP_FALLBACK):
        try:
            status, text = get(url)
            data = json.loads(text)
            ja3 = data.get("ja3_hash") or data.get("ja3") or (
                data.get("tls", {}) or {}
            ).get("ja3_hash")
            ja4 = data.get("ja4") or (data.get("tls", {}) or {}).get("ja4")
            peet = data.get("peetprint_hash") or data.get("peetprint")
            ua_seen = data.get("user_agent") or (
                data.get("http1", {}) or {}
            ).get("user_agent")
            print(f"  [{label}] via {url.split('/')[2]}  status={status}")
            print(f"      ja3_hash   : {ja3}")
            print(f"      ja4        : {ja4}")
            if peet:
                print(f"      peetprint  : {peet}")
            if ua_seen:
                print(f"      UA-as-seen : {ua_seen}")
            return {"ja3": ja3, "ja4": ja4}
        except Exception as e:  # noqa: BLE001
            print(f"  [{label}] {url.split('/')[2]} failed: {e}")
    return None


def _ddg_flow(label, get):
    """Run the real vqd + i.js flow and report block vs. full body."""
    try:
        status, text = get(
            "https://duckduckgo.com/",
            params={"q": _QUERY, "iax": "images", "ia": "images"},
        )
        m = re.search(r'vqd=["\'](\d-[\d-]+)["\']', text) or re.search(
            r'"vqd":"(\d-[\d-]+)"', text
        )
        print(f"  [{label}] landing status={status}  vqd_found={bool(m)}")
        if not m:
            print(f"      -> no vqd token (landing body {len(text)} bytes)")
            return
        params = dict(_IJS_PARAMS, vqd=m.group(1))
        status, text = get(
            "https://duckduckgo.com/i.js",
            params=params,
            headers={"Referer": "https://duckduckgo.com/"},
        )
        body_len = len(text)
        result_count = None
        try:
            result_count = len(json.loads(text).get("results", []))
        except Exception:  # noqa: BLE001 - empty/non-JSON body is the signal
            pass
        if status != 200:
            verdict = f"REJECTED (HTTP {status})"
        elif not result_count:
            verdict = "BLOCKED (200 + empty/non-JSON body)"
        else:
            verdict = f"OK ({result_count} results)"
        print(
            f"  [{label}] i.js status={status}  body={body_len}B  "
            f"results={result_count}  -> {verdict}"
        )
    except Exception as e:  # noqa: BLE001
        print(f"  [{label}] flow error: {e}")


def run_probe():
    print("DuckDuckGo transport diagnostic probe")
    _environment()

    transports = []
    try:
        transports.append(("requests", _requests_transport()))
    except Exception as e:  # noqa: BLE001
        print(f"\n[requests transport unavailable: {e}]")
    try:
        transports.append(("primp-default", _primp_default_transport()))
    except Exception as e:  # noqa: BLE001
        print(f"\n[primp-default transport unavailable: {e}]")
    try:
        primp_get, imp = _primp_impersonate_transport()
        transports.append((f"primp-imp({imp})", primp_get))
    except Exception as e:  # noqa: BLE001
        print(f"\n[primp-impersonate transport unavailable: {e}]")

    _hr("TLS FINGERPRINT (what the server sees)")
    fps = {}
    for label, get in transports:
        fps[label] = _fingerprint(label, get)

    _hr("REAL DDG i.js FLOW (blocked vs full body)")
    for label, get in transports:
        _ddg_flow(label, get)

    _hr("READ-ME")
    print(
        "Three transports are tested per environment:\n"
        "  requests        - what the Linux/direct path uses today\n"
        "  primp-default   - how ddgs 9.x drives HTTP (no impersonation)\n"
        "  primp-imp(...)  - primp forced to impersonate a desktop browser\n"
        "\n"
        "Run this in the environment where the block was actually seen (the\n"
        "frozen Windows setup-python build), not just a modern-OpenSSL dev box\n"
        "where everything already works. The fix is whichever transport shows\n"
        "OK where 'requests' shows BLOCKED/REJECTED. On modern OpenSSL, both\n"
        "requests and primp-default return results and only impersonation gets\n"
        "a 403 - so 'impersonate a browser' is NOT automatically the answer."
    )


if __name__ == "__main__":
    run_probe()
