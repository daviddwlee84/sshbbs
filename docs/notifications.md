# Notifications

The BBS fans out per-user webhook notifications for four event kinds:

| Kind     | Fires when                                                  | Self-event filtered? |
|----------|-------------------------------------------------------------|----------------------|
| `push`   | someone 推 / 噓 / → on one of my articles                   | yes                  |
| `wb`     | someone sends me a 水球 (DM)                                | yes (memo path)      |
| `mail`   | someone sends me a 站內信                                   | yes (memo path)      |
| `reply`  | someone posts a `Re:` reply to one of my articles           | yes                  |

Each user opts in per-event in 主選單 → 個人設定 → 通知設定, and adds
one or more webhook URLs that receive `application/json` POSTs of:

```json
{"title": "[BBS] alice 推了你的文章", "body": "Hello world\n\n→ 推 cool"}
```

There is also an "only when offline" toggle: when on, the dispatcher
checks `chat.Broker.IsOnline(uid)` and skips delivery if the recipient
has at least one live SSH session.

## Architecture decision: webhook indirection, not embedded library

We considered three architectures:

1. **Vendor `unraid/apprise-go`** — pure-Go, no Python, but in v0.1.x
   alpha with no documented Go API. Locks the BBS to its release cadence.
2. **Shell out to `apprise` CLI** — works but adds an external binary
   dependency to the deployment story.
3. **Generic webhook POST (chosen)** — the BBS speaks one protocol
   (`POST` `{title, body}`), and any of the following work without code
   changes:
   - `caronc/apprise-api` (recommended for the full apprise:// surface)
   - ntfy.sh, Discord webhooks, Slack webhooks
   - any custom receiver

This keeps the BBS dependency-light. Operators who want apprise://
URL parsing run `caronc/apprise-api` as a sidecar; everyone else points
the BBS at whatever webhook stack they already have.

## Recommended deployment: bundled docker-compose

`apprise` is already in the canonical [`docker-compose.yml`](../docker-compose.yml)
alongside `sshbbs` and `hostkey-init`, so a single command brings up
the full stack:

```bash
docker compose up -d        # builds sshbbs + pulls apprise + seeds host key
# or, if you prefer the wrapper Make target:
make compose-up
```

What you get:

- `sshbbs` on `:2222` (SSH BBS)
- `apprise` on `:8000` (caronc/apprise-api admin UI + REST endpoints)
- Persistent named volumes: `bbs-data`, `bbs-keys`, `apprise-config`
  — survive `docker compose down`, never lost across restarts.

`make compose-down` stops both services; `make compose-down-volumes`
also wipes the volumes (rare — only when you want a fresh DB and a
fresh apprise config).

## Where do I configure my webhook?

Two pieces, every BBS user does both:

### 1. Register your notification URL(s) in apprise-api

Apprise-api keeps a key/value store of `<key> → [apprise:// URLs]`.
Pick a unique key for yourself (e.g. `alice-discord`, your username
plus the service name, anything unique within this apprise instance)
and POST your apprise:// URL(s) under it:

```bash
curl -X POST http://localhost:8000/add/alice-discord \
  -H 'Content-Type: application/json' \
  -d '{"urls":"discord://webhook_id/webhook_token"}'
```

Multiple services under one key (one webhook hit fans out to all):

```bash
curl -X POST http://localhost:8000/add/alice-all \
  -H 'Content-Type: application/json' \
  -d '{"urls":"discord://...,tgram://bot_token/chat_id,mailto://user:pass@gmail.com"}'
```

Smoke test the apprise → service path before involving the BBS:

```bash
curl -X POST http://localhost:8000/notify/alice-discord \
  -H 'Content-Type: application/json' \
  -d '{"title":"test","body":"hello from cli"}'
```

If your Discord channel pinged, the apprise leg is green and any BBS
notification fired at this key will reach Discord.

The full apprise:// URL catalog (Slack / Telegram / ntfy.sh / SMTP /
Pushover / 100+ more) lives at
<https://github.com/caronc/apprise/wiki>.

> **Tip:** apprise-api also has a browser admin UI at
> <http://localhost:8000>. You can register / inspect / delete keys
> without curl if you prefer.

### 2. Tell BBS where to POST

SSH in (not `guest`), then **主選單 → 個人設定 → 通知設定**:

1. Toggle which events you want delivered: 推/噓 / 水球 / 信箱 / Re:
   回文 / 僅在離線時通知. Space flips, Ctrl+S saves.
2. Press `a` to add a webhook target:
   - **Label**: free-form, your bookkeeping (e.g. "discord").
   - **URL**: `http://apprise:8000/notify/<your-key>` — the docker
     service name `apprise` is what BBS resolves inside the compose
     network. **Not** `localhost`: BBS itself is in a container, so
     `localhost` would mean the BBS container, not the apprise one.
3. Enter to save the target. Test by getting another user to push
   your post (or send yourself mail from a second account).

#### Hostname cheat sheet

The URL you put in 通知設定 depends on where BBS is running:

| BBS runs in…              | Apprise URL the BBS should POST to                |
|---------------------------|----------------------------------------------------|
| docker compose (default)  | `http://apprise:8000/notify/<key>`                 |
| host (`make run`)         | `http://localhost:8000/notify/<key>`               |
| different host / network  | `http://<apprise-host-ip-or-dns>:8000/notify/<key>`|

`t` toggles a target enabled/disabled without losing the row, `e`
edits in place, `d` deletes.

## Payload schema

The BBS sends exactly:

```json
{"title": "<one-line subject>", "body": "<short body or excerpt>"}
```

This matches `apprise-api`'s `/notify/<key>` contract directly. For
non-apprise webhooks (ntfy.sh, Discord) the same shape works because
those services accept arbitrary JSON and template against named fields.

The `Content-Type: application/json` and `User-Agent: sshbbs-notify/1`
headers are set on every request.

## Troubleshooting

| Symptom                                | Where to look                                                   |
|----------------------------------------|------------------------------------------------------------------|
| No notification fires at all           | `~/data/bbs.db` — confirm the row in `user_notif_targets` exists and `enabled = 1`; confirm prefs allow this kind in `user_notif_prefs`. |
| BBS log says `notify: target rejected` | Webhook returned 4xx/5xx. `curl -v` the same URL with the same JSON to see what the receiver complained about. |
| BBS log says `notify: post failed`     | Network error / DNS / TLS. `docker compose logs apprise` to see whether the sidecar is up. |
| Got an apprise-api 200 but no message  | Apprise config key is empty / wrong service URL. `curl http://localhost:8000/get/<key>` lists the URLs the key currently maps to. |
| Notification fires while user is online | `only_when_offline` is unchecked. Tick it in 通知設定. |
| Same event fires twice                 | The user has two enabled targets pointing at the same upstream. Check the target list. |

## Privacy / threat model

- Webhook URLs leave your network with every notification. For sensitive
  content (private boards, mail bodies), prefer **self-hosted** apprise-api
  over public webhook services.
- Webhook URLs are stored in plaintext in `data/bbs.db`. Treat that file
  as a secret — it can also be used to send arbitrary messages on the
  user's behalf if compromised.
- The notification body excerpts up to ~280 characters of mail body /
  push comment. If you don't want that on third-party servers, route
  through self-hosted apprise-api configured to drop body and only
  forward titles.
