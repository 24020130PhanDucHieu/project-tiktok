#!/usr/bin/env bash
# Copies files to a server over ssh, one `cat` per file.
#
# Why not scp: scp (and sftp) speak their protocol over the same stdout the server's login shell writes to. A server that
# prints anything when a session opens (neofetch, a banner, an echo in ~/.bashrc) corrupts that stream, and scp dies with
# "Received message too long" and exit code 255. Here the file only travels on stdin, so whatever the server prints is ignored.
#
# Like scp, it overwrites the file in place: a file that already exists keeps the mode the server gave it.
#
# Usage: ship-files.sh <ssh-key> <user@host> <remote-dir> <file>[:<name-on-the-server>]...
set -euo pipefail

if [ "$#" -lt 4 ]; then
	echo "usage: $0 <ssh-key> <user@host> <remote-dir> <file>[:<remote-name>]..." >&2
	exit 2
fi

key=$1
host=$2
dir=$3
shift 3

for spec in "$@"; do
	src=${spec%%:*}
	name=${spec#*:}
	[ "$name" = "$spec" ] && name=$(basename "$src")
	if [ ! -f "$src" ]; then
		echo "ship-files: no such file: $src" >&2
		exit 1
	fi
	# stdout of ssh is thrown away on purpose: it only holds what the server's login shell prints; errors still show on stderr
	ssh -i "$key" -o BatchMode=yes "$host" "cat > '$dir/$name'" <"$src" >/dev/null
	echo "shipped $src -> $dir/$name"
done
