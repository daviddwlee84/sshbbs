#!/usr/bin/env bash
# Records docs/demo.gif using VHS.
#
# Flow:
#   1. Kill any running sshbbs and wipe data/bbs.db.
#   2. Build the server.
#   3. Boot the server, register alice (pw123456) via expect, seed two
#      articles in the Test board, restart the server clean.
#   4. Run `vhs scripts/demo.tape` which drives a real ssh client.
#   5. Tear everything down.
#
# Run from the repo root.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DB="$ROOT/data/bbs.db"
HOSTKEY="$ROOT/.ssh/host_ed25519"
BIN="$ROOT/sshbbs"
TAPE="$ROOT/scripts/demo.tape"

cleanup() {
    if [[ -n "${SRV_PID:-}" ]] && kill -0 "$SRV_PID" 2>/dev/null; then
        kill "$SRV_PID" 2>/dev/null || true
        wait "$SRV_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Belt-and-braces: kill anything else that's bound to :2222.
pkill -f "$BIN" 2>/dev/null || true
sleep 0.3

# Reset DB.
rm -f "$DB" "$DB-shm" "$DB-wal" "$DB-journal"

# Build.
echo "==> building"
go build -o "$BIN" ./cmd/sshbbs

# Generate host key if missing.
if [[ ! -f "$HOSTKEY" ]]; then
    bash "$ROOT/scripts/gen-hostkey.sh" "$HOSTKEY"
fi

# Boot server, register alice, seed articles.
echo "==> bootstrapping demo state"
"$BIN" -addr=:2222 -db="$DB" -hostkey="$HOSTKEY" >/tmp/sshbbs-demo.log 2>&1 &
SRV_PID=$!
sleep 1

expect <<'EOF' >/dev/null
set timeout 5
spawn ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 2222 new@localhost
expect "password:"
send "any\r"
expect -re "註冊新帳號"
send "alice\tpw123456\tAlice\tdemo@example.com\r"
expect -re "註冊成功"
send "\r"
expect eof
EOF

sqlite3 "$DB" <<SQL
INSERT INTO articles (board_id, author_id, author_userid, title, body) VALUES
    (2, 1, 'alice', '歡迎使用 SSH-BBS',
     '本站特色：vim 風格導覽 (h j k l)。' || char(10) ||
     'h / ← / Esc 後退；l / → / Enter 前進。' || char(10) ||
     '推文 + - = 對應 推 / 噓 / →。' || char(10) ||
     'Ctrl+U 隨時打開水球收件夾。'),
    (2, 1, 'alice', '推文示範文章',
     '在這裡按 + 推、按 - 噓、按 = 補充箭頭。' || char(10) ||
     '其他連線中的使用者會即時看到你的推文。');
SQL

echo "==> articles seeded:"
sqlite3 "$DB" "SELECT id, title FROM articles;"

echo "==> recording with vhs"
mkdir -p "$ROOT/docs"
vhs "$TAPE"

echo "==> done — output at docs/demo.gif"
ls -la "$ROOT/docs/demo.gif"
