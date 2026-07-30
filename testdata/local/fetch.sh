#!/bin/sh
# Fetch the story files this repository cannot ship, for local testing only.
#
# Only the zork1, zork2, and zork3 repositories in historicalsource carry a
# licence. The stories below do not, which is why they are downloaded into a
# gitignored directory rather than committed. See README.md and D43 in
# ../../spec-deltas.md.
#
# Requires the gh CLI, authenticated. Run from this directory.

set -eu

cd "$(dirname "$0")"

fetch() {
	repo=$1
	path=$2
	out=$3

	if [ -f "$out" ]; then
		printf '%-16s already present\n' "$out"
		return
	fi
	printf '%-16s fetching from historicalsource/%s\n' "$out" "$repo"
	gh api "repos/historicalsource/$repo/contents/$path" \
		-H "Accept: application/vnd.github.raw" >"$out"
}

fetch borderzone  COMPILED/spy.z5      spy.z5
fetch journey     COMPILED/journey.z6  journey.z6
fetch beyondzork  COMPILED/bzbeta.z5   bzbeta.z5

# Report what arrived, and check each file against its own header, so that a
# truncated download or a replaced file is obvious now rather than later.
printf '\n'
for f in spy.z5 journey.z6 bzbeta.z5; do
	[ -f "$f" ] || continue
	python3 - "$f" <<'PY'
import sys

name = sys.argv[1]
d = open(name, 'rb').read()
if len(d) < 0x40:
    print(f"{name}: too short to be a story image")
    sys.exit(1)

version = d[0]
scale = {1: 2, 2: 2, 3: 2, 4: 4, 5: 4}.get(version, 8)
release = int.from_bytes(d[2:4], 'big')
serial = d[0x12:0x18].decode('latin1')
declared = int.from_bytes(d[0x1a:0x1c], 'big') * scale
stored = int.from_bytes(d[0x1c:0x1e], 'big')

if not 0 < declared <= len(d):
    print(f"{name}: declares {declared} bytes but is {len(d)}; the download looks incomplete")
    sys.exit(1)

computed = sum(d[0x40:declared]) & 0xffff
ok = "ok" if computed == stored else f"MISMATCH (computed {computed:#06x})"
print(f"{name}: version {version}, release {release}, serial {serial}, "
      f"{len(d)} bytes, checksum {stored:#06x} {ok}")
PY
done
