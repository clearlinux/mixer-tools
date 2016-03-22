#!/bin/sh

set -e

autoreconf --force --install --symlink --warnings=all

args="\
--prefix=/usr \
--program-prefix=mixer-"

if test -z "${NOCONFIGURE}"; then
  ./configure $args "$@"
  make clean
fi
