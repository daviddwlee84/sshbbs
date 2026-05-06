# Notifications

Per-user webhook fan-out for four event kinds. Each user opts in via
**主選單 → 個人設定 → 通知設定** and configures one or more URLs that
receive the notifications.

| Kind     | Fires when                                                  | Self-event filtered? |
|----------|-------------------------------------------------------------|----------------------|
| `push`   | someone 推 / 噓 / → on one of my articles                   | yes                  |
| `wb`     | someone sends me a 水球 (DM)                                | yes (memo path)      |
| `mail`   | someone sends me a 站內信                                   | yes (memo path)      |
| `reply`  | someone posts a `Re:` reply to one of my articles           | yes                  |

There is also an "only when offline" toggle: when on, the dispatcher
checks `chat.Broker.IsOnline(uid)` and skips delivery if the user has at
least one live SSH session.

## How it works

The BBS POSTs JSON to whatever URL each user configures. The exact JSON
shape depends on the URL — the dispatcher picks an encoder by URL prefix:

- **Discord webhook URL** (`https://discord.com/api/webhooks/...`,
  including `discordapp.com` / `canary` / `ptb` variants) → embed-wrapped
  payload that Discord renders as a structured message:

    ```json
    {
      "username": "SSH-BBS",
      "embeds": [{"title": "...", "description": "...", "color": 10317823}]
    }
    ```

- **Anything else** → flat `{title, body}` apprise-style JSON:

    ```json
    {"title": "[BBS] alice 推了你的文章", "body": "→ 推 nice post"}
    ```

  This shape is what `caronc/apprise-api`'s `/notify/<key>` endpoint
  expects. ntfy.sh, custom webhook receivers, and most generic
  integrations also handle it cleanly.

Adding a new direct integration (Slack incoming webhooks, Telegram bot
HTTP, …) means adding one URL pattern + one encoder in
`internal/notify/encoder.go` — no schema or UI change.

## Picking a target

Most BBS users want one of these. Pick the easiest one that hits a
device you already carry.

| Want                                          | Use                                                         | ~setup time |
|-----------------------------------------------|-------------------------------------------------------------|-------------|
| Discord channel ping                          | Discord channel webhook URL — paste it directly             | 30 s        |
| Push notification on my phone                 | ntfy.sh topic URL (free public, or self-host)               | 1 min       |
| Multiple services fan-out (Discord + email + ...) | Self-hosted apprise-api on a machine you control            | 5 min       |
| Internal Slack channel                        | Slack incoming webhook URL — POSTs `{title, body}` as text  | 2 min       |
| Custom logic (filter, templating, archive)    | Your own n8n / Zapier / IFTTT / handcrafted endpoint        | varies      |

Whatever you pick: the only thing the BBS needs is **an HTTP(S) URL it
can POST to**. The user (you) own the credential / API key embedded in
that URL, and the BBS operator can see it (see "Operator's view" below).

### Walkthrough A: Discord webhook (direct)

1. In Discord: server settings → **Integrations** → **Webhooks** →
   **New Webhook** → pick the channel → **Copy Webhook URL**.
   The URL looks like `https://discord.com/api/webhooks/<id>/<token>`.
2. SSH into the BBS, **主選單 → 個人設定 → 通知設定 → `a` 新增 target**:
   - **Label**: `discord` (just for your bookkeeping)
   - **URL**: paste the Discord webhook URL verbatim
3. Tick the events you want (推/噓 / 水球 / 信箱 / Re:), `Ctrl+S` to save.
4. Have a friend push your article (or send yourself mail from a second
   account) → expect the embed-wrapped notification in Discord within
   a second or two.

The BBS automatically detects the Discord URL prefix and switches to
the embed envelope — no flag, no format selector.

### Walkthrough B: ntfy.sh (push notification on phone)

1. Install the [ntfy app](https://ntfy.sh/) on iOS / Android, or use
   the web UI.
2. Subscribe to a topic — pick something unguessable, e.g.
   `bbs-alice-7f3a91`. Topic names are public-by-design, so anyone who
   guesses yours can read it. Use a long random suffix.
3. Add target in BBS:
   - **URL**: `https://ntfy.sh/bbs-alice-7f3a91`
4. ntfy renders the `{title, body}` JSON as a notification with the
   title as the heading and body as the message text.

For self-hosted ntfy with auth, use
`https://<user>:<password>@ntfy.example.com/<topic>`.

### Walkthrough C: Self-hosted apprise-api (multi-service fan-out)

If you want one webhook hit to fan out to Discord + Telegram + email
+ SMS, run [`caronc/apprise-api`](https://github.com/caronc/apprise-api)
on a machine you control (NAS, VPS, homelab). The repo's bundled
demo profile is one option (see "Demo profile" below); production
deployments should run their own.

1. On your machine: `docker run -d -p 8000:8000 -v ./config:/config caronc/apprise:latest`
2. Register your apprise:// URLs under a key:

    ```bash
    curl -X POST http://your-host:8000/add/mykey \
      -H 'Content-Type: application/json' \
      -d '{"urls":"discord://<id>/<token>,tgram://<bot>/<chat>,mailto://user:pass@gmail.com"}'
    ```

3. Add target in BBS:
   - **URL**: `http://your-host:8000/notify/mykey`
4. Apprise translates `{title, body}` into each service's native format.

The full apprise:// URL catalog (Slack, Telegram, ntfy.sh, SMTP,
Pushover, 100+ more) is at <https://github.com/caronc/apprise/wiki>.

## BBS-side configuration (same for every target type)

Inside 通知設定:

- **Toggle list (top half)**: Space flips a row. Ctrl+S persists. The
  five rows: 推/噓 / 水球 / 信箱 / Re: 回文 / 僅在離線時通知.
- **Target list (bottom half)**: each row is one URL.
  - `a` add (inline form)
  - `e` edit (URL or label)
  - `t` toggle enabled (preserves the row, just stops sending)
  - `d` delete
- **URL hostname cheat sheet** (only matters for the bundled demo
  apprise instance — direct webhooks like Discord don't have this
  problem since they're public URLs):

    | BBS runs in…              | Demo apprise URL                                  |
    |---------------------------|---------------------------------------------------|
    | docker compose with `--profile demo` | `http://apprise:8000/notify/<key>`     |
    | host (`make run`)         | `http://localhost:8000/notify/<key>`              |
    | different host / network  | `http://<apprise-host>:8000/notify/<key>`         |

## Demo profile: bundled apprise-api

The repo's [`docker-compose.yml`](../docker-compose.yml) ships a
`caronc/apprise-api` service under the `demo` profile. **It is not
started by default.** Use it for:

- single-user homelab deployments (you = operator = only user)
- local development
- demos / smoke testing — see [`cmd/bbsmoke`](../cmd/bbsmoke/main.go)
  for the end-to-end harness this PR was developed against

```bash
docker compose --profile demo up -d   # brings up apprise + sshbbs + hostkey-init
make compose-down                     # stops everything
```

**Do not use the bundled apprise on a multi-tenant BBS.** Reasons in
"Operator's view" below — the short version is its admin REST API on
:8000 has no authentication, so any user with network reachability can
read or delete every other user's keys. Production multi-user
deployments should either (a) point each user at their own apprise
instance, (b) point each user at a direct webhook (Discord/ntfy/etc.),
or (c) run apprise-api with its own authentication proxy in front.

## Operator's view (privacy / threat model)

What the BBS operator can see:

- **The target URL each user added.** This URL itself is the
  credential — a Discord webhook URL grants send-as-this-channel; an
  apprise-api URL exposes the apprise key. Treat `data/bbs.db` as
  containing user secrets. Backup / RBAC / encryption-at-rest decisions
  should match.
- **Every notification's title and body excerpt** that left the BBS,
  via debug logs (`charmbracelet/log` at warn-or-above by default;
  bump to debug only when investigating).

What the operator does **not** see:

- The downstream service credentials (Discord token / Telegram bot
  token / SMTP password) — those live wherever the user pointed the
  webhook URL, never on the BBS.

What users should know:

- Notification body excerpts up to ~280 characters of mail body / push
  comment. If you don't want even that on the receiving service, route
  through self-hosted apprise-api configured to drop body and only
  forward titles.
- Webhook URLs can be revoked (regenerated in Discord / rotated in
  apprise) at any time; the BBS will start logging `notify: target
  rejected` 4xx until the user updates the URL in 通知設定.

## Payload schema reference

### Default ({title, body} for apprise-api / ntfy / generic)

```json
{"title": "[BBS] bob 推了你的文章 «Hello»", "body": "→ 推 nice post"}
```

Headers: `Content-Type: application/json`, `User-Agent: sshbbs-notify/1`.

### Discord embed (when URL prefix matches `discord.com/api/webhooks/`)

```json
{
  "username": "SSH-BBS",
  "embeds": [{
    "title": "[BBS] bob 推了你的文章 «Hello»",
    "description": "→ 推 nice post",
    "color": 10317823
  }]
}
```

Limits: title ≤ 256 runes, description ≤ 4096 runes — both truncated
with `…` rather than byte-cut, so CJK glyphs aren't corrupted.

## What's logged on the server

Every dispatch leaves a trail in the BBS log. URLs are redacted to
hide the credential-bearing last path segment (Discord token / Slack
signing key / ntfy topic / apprise config key).

| Level | Message                       | Fired when                                          |
|-------|-------------------------------|-----------------------------------------------------|
| DEBUG | `notify: queued`              | Event accepted into the dispatch queue              |
| INFO  | `notify: delivered`           | 2xx/3xx — success, includes target ID + duration    |
| INFO  | `notify: test delivered`      | `T 測試` succeeded                                  |
| WARN  | `notify: queue full, dropping event` | Queue full (slow upstream + many concurrent events) |
| WARN  | `notify: build request`       | Encoder failed (very unusual — JSON marshal error)  |
| WARN  | `notify: post failed`         | Transport-layer failure (DNS / TCP / TLS / timeout) |
| WARN  | `notify: target rejected`     | 4xx/5xx from receiver                               |
| WARN  | `notify: test post failed`    | Same as `post failed` but during `T 測試`           |
| WARN  | `notify: test target rejected`| Same as `target rejected` but during `T 測試`       |

Tail the server log while triggering events to confirm where in the
chain something is breaking. Successes log at INFO so they show up
under the default level; the queue entry is DEBUG-only to avoid
flooding under load.

## Troubleshooting

| Symptom                                | Where to look                                                   |
|----------------------------------------|------------------------------------------------------------------|
| `T 測試` fails with `context deadline exceeded (Client.Timeout exceeded while awaiting headers)` on macOS | DNS poisoning + Clash for Windows proxy mismatch. macOS GUI proxy doesn't propagate to Go binaries. Set `HTTPS_PROXY=http://127.0.0.1:7891` (or your Clash port) plus `NO_PROXY=localhost,127.0.0.1,...` in the shell that runs BBS. Full diagnosis + commands in [`pitfalls/notify-context-deadline-exceeded-while-awaiting-headers.md`](../pitfalls/notify-context-deadline-exceeded-while-awaiting-headers.md). |
| No notification fires, no log either   | Pref toggle for that event kind is off. Check 個人設定 → 通知設定 top half. |
| BBS log says `notify: target rejected` (4xx) | Webhook returned an error. `curl -v` the same URL with the same JSON shape to see the receiver's complaint. Common Discord 400: payload didn't match the embed schema — auto-detect should prevent this, file a bug if you see it. |
| BBS log says `notify: post failed`     | Network error / DNS / TLS. Run the macOS-proxy diagnostic above first if applicable; otherwise `curl -v <target-url>` with a sample payload from the BBS host. |
| Discord receives nothing but apprise-style URL works | URL probably doesn't match the auto-detect prefix. Confirm it starts with `https://discord.com/api/webhooks/` — variants like `discordapp.com` / `canary.discord.com` / `ptb.discord.com` are also detected. A CDN proxy in front of Discord would break detection — point directly at Discord. |
| Notification fires while user is online | `only_when_offline` is unchecked. Tick it in 通知設定. |
| Same event fires twice                 | Two enabled targets pointing at the same upstream. Check the target list. |
| Webhook token visible in error message / screenshot | `RedactURL` (this PR) hides the last path segment from logs and the flash row. If you see a full token, you're on a build older than 22e4f63 — pull and rebuild. |

## Architecture decision: webhook indirection, not embedded library

We considered three architectures:

1. **Vendor `unraid/apprise-go`** — pure-Go, no Python, but in v0.1.x
   alpha with no documented Go API. Locks the BBS to its release cadence.
2. **Shell out to `apprise` CLI** — works but adds an external binary
   dependency to the deployment story.
3. **Generic webhook POST (chosen)** — the BBS speaks one protocol
   (HTTP POST JSON), and the URL prefix selects the encoder. Direct
   integrations (Discord) get native payloads; everything else gets
   the apprise-style envelope. No library lock-in, no required sidecar.

The bundled `caronc/apprise-api` in the `demo` compose profile is a
convenience for single-user homelab cases, not the recommended path
for multi-user deployments.
