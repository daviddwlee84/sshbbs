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

## Recommended deployment: docker-compose with apprise-api

Drop this `docker-compose.yml` (or merge into your existing one) at the
repo root:

```yaml
services:
  apprise:
    image: caronc/apprise:latest
    container_name: bbs-apprise
    ports: ["8000:8000"]
    volumes: ["./apprise/config:/config"]
    restart: unless-stopped

  bbs:
    build: .
    ports: ["2222:2222"]
    volumes:
      - ./data:/app/data
      - ./.ssh:/app/.ssh
    depends_on: [apprise]
```

A working example lives at [`docker-compose.example.yml`](../docker-compose.example.yml).

Bring it up:

```bash
docker compose -f docker-compose.example.yml up -d
```

## First-time apprise-api setup

`caronc/apprise-api` stores notification URLs under named "config keys".
Add Discord (or any apprise-supported service) under key `mykey`:

```bash
curl -X POST http://localhost:8000/add/mykey \
  -H 'Content-Type: application/json' \
  -d '{"urls": "discord://webhook_id/webhook_token"}'
```

Verify it works end-to-end:

```bash
curl -X POST http://localhost:8000/notify/mykey \
  -H 'Content-Type: application/json' \
  -d '{"title": "test", "body": "hello from cli"}'
```

If your Discord channel pinged, the path BBS → apprise → Discord is
green.

## Per-user BBS configuration

1. SSH in, log in (not `guest`).
2. From the main menu pick **個人設定 User settings → 通知設定**.
3. Toggle which events you want delivered (Space flips, Ctrl+S saves).
4. Press `a` to add a webhook target. Two fields:
   - **Label**: free-form, just for your own bookkeeping.
   - **URL**: `http://apprise:8000/notify/mykey` (inside the docker
     network) or `http://localhost:8000/notify/mykey` (from the host).
5. Trigger the event (have a friend push your post, send yourself a
   水球 from another account, etc.) and watch the notification land.

`t` toggles a target enabled/disabled without losing the row, `e` edits
in place, `d` deletes.

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
