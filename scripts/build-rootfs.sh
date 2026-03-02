#!/bin/bash
set -euo pipefail

IMAGE="${1:?usage: build-rootfs.sh <docker-image> [output] [size]}"
OUTPUT="${2:-out/rootfs.ext4}"
SIZE="${3:-512M}"

cid=""
staging="$(mktemp -d)"
trap 'if [ -n "${cid-}" ]; then docker rm -f "${cid}"; fi; rm -rf "${staging}"' EXIT

cid=$(docker create "${IMAGE}" /bin/true)
docker export "${cid}" | tar -xf - -C "${staging}"

mkdir -p "$(dirname "${OUTPUT}")"
truncate -s "${SIZE}" "${OUTPUT}"
mkfs.ext4 -q -d "${staging}" "${OUTPUT}"
