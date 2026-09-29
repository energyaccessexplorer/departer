#!/bin/sh
#
# departer-deploy.sh — install the binary the CI shipped and restart the service.
#
# Mirrors paver's arrangement: this script lives root-owned at
# /usr/local/sbin/departer-deploy.sh and the restricted CI user may run exactly
# this through sudo, nothing else:
#
#   departerci ALL=(root) NOPASSWD: /usr/local/sbin/departer-deploy.sh
#
# CI ships the binary to ~departerci/incoming/departer (see
# .github/workflows/production-deploy.yml) and then runs this via
# `ssh departerci@… sudo /usr/local/sbin/departer-deploy.sh`.

set -eu

INCOMING=${1:-/home/departerci/incoming/departer}
TARGET=/usr/local/bin/departer

[ -f "$INCOMING" ] || { echo "no binary at $INCOMING" >&2; exit 1; }

# Keep the outgoing binary: rollback is `install`ing it back + restart.
[ -f "$TARGET" ] && cp -a "$TARGET" "${TARGET}.prev"

install -o root -g root -m 755 "$INCOMING" "$TARGET"
rm -f "$INCOMING"

systemctl restart departer.service

# The unit's ExecStartPre removes the socket and the service recreates it.
for _ in $(seq 1 10); do
	[ -S /tmp/departer-server.sock ] && exit 0
	sleep 1
done

echo "departer did not come back up on /tmp/departer-server.sock" >&2
systemctl --no-pager --lines=20 status departer.service >&2 || true
exit 1
