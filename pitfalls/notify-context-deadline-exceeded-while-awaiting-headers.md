---
title: 通知設定 T 測試一律 timeout / `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
date: 2026-05-06
---

## Symptom

In 個人設定 → 通知設定 → press `T` to test a freshly added webhook
target. After exactly the dispatcher timeout (5 s by default), the
flash row prints:

```
⚠ target #2 test failed: Post "https://discord.com/api/webhooks/.../...":
context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

Same target URL works fine when:

- pasted into Safari / Chrome
- POSTed via `curl https://discord.com/api/webhooks/...` from the host shell

But fails consistently from the BBS Go process. `make compose-up` (BBS
inside docker) shows the same failure.

A real BBS event (someone pushes your post, sends mail) triggers the
same warning in the server log:

```
WARN notify: post failed target=2 url=https://... kind=push err=...context deadline exceeded...
```

## Root cause

Three layers stack to break Go-on-macOS outbound HTTPS:

1. **DNS-poisoned upstream.** macOS's resolver chain (mDNSResponder, or
   the resolver Go's PureGo path reads from `/etc/resolv.conf`) returns
   poisoned IPs for `discord.com` — examples observed:
   `199.59.149.205` (Bodis parking), `199.16.158.8`, `108.160.162.115`.
   None of these are real Discord IPs (Discord fronts via Cloudflare;
   real answers are `162.159.x.x` etc.). `curl --resolve` to those
   IPs hangs at TCP connect.
2. **Clash for Windows running on macOS proxies HTTPS via `127.0.0.1:7891`**,
   and macOS GUI sets `Server: 127.0.0.1 Port: 7891` under
   System Settings → Network → Wi-Fi → Proxies → Secure Web Proxy.
3. **Go's `http.DefaultTransport.ProxyFromEnvironment` only reads env
   vars** (`HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, `NO_PROXY`). It does
   NOT read macOS GUI proxy settings via `SystemConfiguration` /
   `CFNetwork`. Safari / Chrome use Foundation APIs and pick up the GUI
   setting automatically; Go binaries don't.

Net effect: BBS bypasses Clash, tries to connect directly, hits the
poisoned DNS, TCP connect (or TLS handshake) silently retransmits, the
5-second `http.Client.Timeout` fires before any response headers arrive
— hence the specific "while awaiting headers" wording from
`net/http`.

The error message from `net/http` carries the full URL including the
Discord webhook token. **Treat such logs and screenshots as
secret-bearing** until URL-redaction (this PR) lands everywhere.

## Detection

Any of these from the BBS host quickly tells you DNS is poisoned and
proxy is needed:

```bash
# 1. Resolver returns garbage IP
dig discord.com +short        # → 199.59.149.205 (parking) or similar
dig @1.1.1.1 discord.com +short   # may also be intercepted on this network

# 2. Direct curl hangs
time curl -sS -o /dev/null --max-time 6 https://discord.com/api/v10/gateway
# → Connection timed out after 6000 ms, time_connect=0

# 3. Clash proxy is up
nc -z -w1 127.0.0.1 7891 && echo "clash listening"
networksetup -getsecurewebproxy Wi-Fi    # GUI proxy enabled = Yes

# 4. Same curl with HTTPS_PROXY works fast
time HTTPS_PROXY=http://127.0.0.1:7891 curl -sS -o /dev/null \
  --max-time 6 https://discord.com/api/v10/gateway
# → status=200, time_total ≈ 1s

# 5. Confirm Go inherits the same blockage
cat > /tmp/dnstest.go <<'EOF'
package main
import ("fmt"; "net/http"; "time")
func main() {
    c := &http.Client{Timeout: 6 * time.Second}
    t0 := time.Now()
    resp, err := c.Get("https://discord.com/api/v10/gateway")
    if err != nil { fmt.Printf("err after %v: %v\n", time.Since(t0), err); return }
    resp.Body.Close()
    fmt.Printf("status=%d time=%v\n", resp.StatusCode, time.Since(t0))
}
EOF
go run /tmp/dnstest.go                                # → err: context deadline exceeded
HTTPS_PROXY=http://127.0.0.1:7891 go run /tmp/dnstest.go  # → status=200
```

## Workaround

Set proxy env vars in the shell that launches BBS (also covers
`make run`, `make watch`, any `go run`):

```bash
export HTTPS_PROXY=http://127.0.0.1:7891
export HTTP_PROXY=http://127.0.0.1:7891
export ALL_PROXY=http://127.0.0.1:7891
# Localhost / loopback / Tailscale must NOT go through Clash, or BBS
# loses its own apprise sidecar, its own SSH listener, and any tailnet
# peers (the proxy will refuse to route RFC1918 / loopback).
export NO_PROXY=localhost,127.0.0.1,*.local,*.tailscale.ts.net,100.64.0.0/10
```

Persist by appending to `~/.zshrc`. To verify post-source:

```bash
source ~/.zshrc
echo $HTTPS_PROXY                                    # must be set
T 測試 in 個人設定 → 通知設定 should now succeed in <1s.
```

When BBS runs inside docker (`docker compose up -d`, the canonical
flow), Docker Desktop's vpnkit handles DNS differently and may not
need the env var — verify per-deployment. To pass the env var into
the container:

```yaml
services:
  sshbbs:
    environment:
      - HTTPS_PROXY=http://host.docker.internal:7891
      - NO_PROXY=localhost,127.0.0.1,apprise
```

## Prevention

The dispatcher cannot self-heal this — it's a macOS / Go interaction
outside the BBS's control. Mitigations baked into the BBS instead:

- `internal/notify/encoder.go:RedactURL` strips the last path
  segment (Discord token / Slack signing key / ntfy topic / apprise
  key) from URLs before they hit logs or error messages, so
  screenshotting a failed `T 測試` no longer leaks the token.
- `internal/notify/dispatcher.go` logs the *redacted* URL on both
  success (`INFO notify: delivered`) and failure
  (`WARN notify: post failed`), so users can grep their server log to
  distinguish "BBS never tried" from "BBS tried but receiver
  rejected".

Cross-link: deeper rationale lives in `docs/notifications.md` →
"Troubleshooting" (the proxy-on-macOS row).

## Related

- [`pitfalls/water-balloon-self-send-hangs-server.md`](water-balloon-self-send-hangs-server.md)
  — different bug, same general lesson (BBS is one HTTP client among
  many on the host; assumptions baked into shells / GUIs don't
  automatically reach Go).
- Go upstream discussion of macOS GUI proxy support:
  [golang/go#26439](https://github.com/golang/go/issues/26439) —
  open since 2018, no first-party `mac-proxy` resolver in `net/http`.
