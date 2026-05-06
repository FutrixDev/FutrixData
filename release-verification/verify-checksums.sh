#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <directory-containing-SHA256SUMS.txt>" >&2
  exit 2
fi

cd "$1"
if [ ! -f SHA256SUMS.txt ]; then
  echo "missing SHA256SUMS.txt" >&2
  exit 1
fi

shasum -a 256 -c SHA256SUMS.txt
