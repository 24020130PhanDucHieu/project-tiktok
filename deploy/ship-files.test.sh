#!/usr/bin/env bash
# Tests ship-files.sh against a fake `ssh` that behaves like a server whose login shell prints a banner (the situation
# that broke scp). Run:  bash deploy/ship-files.test.sh
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# a fake ssh: prints a banner that starts with spaces (like neofetch), then runs the command it was given, locally
mkdir -p "$work/bin" "$work/remote"
cat >"$work/bin/ssh" <<'FAKE'
#!/usr/bin/env bash
cmd=${*: -1}
printf '      `-/+++++++++++/-.\n   OS: Oracle Linux\n'
exec bash -c "$cmd"
FAKE
chmod +x "$work/bin/ssh"
export PATH="$work/bin:$PATH"

printf 'services: {}\n' >"$work/compose.yml"
printf 'caddy config with spaces\n' >"$work/Caddyfile"
printf '#!/bin/sh\necho ok\n' >"$work/maintenance.sh"
key=$work/key

# 1. the files arrive intact although the server prints a banner on every connection
bash "$here/ship-files.sh" "$key" opc@host "$work/remote" "$work/compose.yml" "$work/Caddyfile" "$work/maintenance.sh" >"$work/out.txt"
for f in compose.yml Caddyfile maintenance.sh; do
	cmp -s "$work/$f" "$work/remote/$f" || fail "$f did not arrive intact"
done
grep -q "Oracle Linux" "$work/out.txt" && fail "the server's banner leaked into the output"

# 2. a file can be given another name on the server
bash "$here/ship-files.sh" "$key" opc@host "$work/remote" "$work/compose.yml:docker-compose.prod.yml" >/dev/null
cmp -s "$work/compose.yml" "$work/remote/docker-compose.prod.yml" || fail "the renamed file did not arrive"

# 3. an existing file keeps the mode the server gave it (scp did the same), and gets the new content
chmod 755 "$work/remote/maintenance.sh"
printf '#!/bin/sh\necho new\n' >"$work/maintenance.sh"
bash "$here/ship-files.sh" "$key" opc@host "$work/remote" "$work/maintenance.sh" >/dev/null
cmp -s "$work/maintenance.sh" "$work/remote/maintenance.sh" || fail "the content was not replaced"
[ -x "$work/remote/maintenance.sh" ] || fail "the executable bit was lost"

# 4. a missing file stops the deploy with an error, and so does a failing ssh
if bash "$here/ship-files.sh" "$key" opc@host "$work/remote" "$work/missing.txt" >/dev/null 2>&1; then fail "a missing file was not reported"; fi
cat >"$work/bin/ssh" <<'DOWN'
#!/usr/bin/env bash
exit 255
DOWN
if bash "$here/ship-files.sh" "$key" opc@host "$work/remote" "$work/compose.yml" >/dev/null 2>&1; then fail "an unreachable server was not reported"; fi

# 5. wrong usage
if bash "$here/ship-files.sh" onlyone >/dev/null 2>&1; then fail "wrong usage was accepted"; fi

echo "ship-files: all checks passed"
