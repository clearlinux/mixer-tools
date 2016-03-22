#!/bin/bash

set -e

CLRVER=
MIXVER=

if [ -e /usr/lib/os-release ]; then
  CLRVER=$(awk -F= '/^VERSION_ID/ { print $2 }' /usr/lib/os-release)
fi

# For a vanilla system, default to "10" for the mix version, since "0" has
# special meaning for SWUPD (zero packs, etc.). Otherwise, add 10 to the latest
# mix version.
if [ -d /var/lib/update/image ]; then
  versions=$(ls /var/lib/update/image/ | grep -E '^[0-9]+' | sort -n)
  if [ -n "$versions" ]; then
    latest=$(echo "$versions" | tail -1)
    MIXVER=$(expr $latest + 10)
  else
    MIXVER=10
  fi
else
  MIXVER=10
fi

usage() {
  echo -n "Usage: $0"
  echo -n " [-h | --help]"
  echo -n " [-c <ver> | --clear-version <ver>]"
  echo -n " [-m <ver> | --mix-version <ver>]"
  echo
}

while [ "$1" != "" ]; do
  case "$1" in
    -c | --clear-version )
      shift; CLRVER="$1"
      ;;
    -m | --mix-version )
      shift; MIXVER="$1"
      ;;
    -h | --help )
      usage; exit 1
      ;;
    * )
      usage; exit 1
      ;;
  esac
  shift
done

if [ -z "$CLRVER" ]; then
  echo "Please specify a Clear version with -c"
  exit 1
fi

if [ -z "$MIXVER" ]; then
  echo "Please specify a mix version with -m"
  echo "The version must be >= 10, and a multiple of 10"
  exit 1
fi

# CLRVER and MIXVER must only be positive integers that are multiples of 10
regexp='^[0-9]+0$'
if ! [[ "$CLRVER" =~ $regexp ]]; then
  echo "CLRVER must be a positive integer and a multiple of 10"
  exit 1
fi
if ! [[ "$MIXVER" =~ $regexp ]]; then
  echo "MIXVER must be a positive integer and a multiple of 10"
  exit 1
fi

# used in other mixer scripts
echo "$CLRVER" > "$PWD/.clear-version"
echo "$MIXVER" > "$PWD/.mix-version"

echo "Initialized Clear version to $CLRVER"
echo "Initialized mix version to $MIXVER"

exit 0

# vi: ts=8 sw=2 sts=2 et tw=80
