#!/bin/bash

BUILDERSCRIPT="bundle-chroot-builder.py"
CLRVER=$(cat "$PWD/.clear-version")
MIXVER=$(cat "$PWD/.mix-version")

# FIXME: For now, only build chroots for the mix. In the future, when we run the
# ABI checker and build_comp to catch additional problems compared to upstream
# Clear, we will need to build vanilla Clear chroots as well.
BUILDTYPE="mix"

# FIXME: LANG is set correctly as root user, but not otherwise.
# Desperately needs a fix in Clear...
export LANG="en_US.utf8"

unset http_proxy
unset https_proxy

check_dep() {
  type $1 &> /dev/null
  if [ $? -ne 0 ]; then
    echo "$1 program not found... Unable to continue"
    exit 1
  fi
}

# Check dependencies

# check_dep "abi-compliance-checker"
# check_dep "bundle-chroot-builder"
check_dep "cp"
check_dep "hardlink"
check_dep "m4"
check_dep "rpm"
check_dep "yum"

if [ ! -e "$PWD/yum.conf.in" ]; then
  cp /usr/share/defaults/mixer/yum.conf.in .
fi

if [ "$BUILDTYPE" = "clear" ]; then
  m4 yum.conf.in > "$PWD/.yum-clear.conf"
  SWUPD_YUM_CONF="$PWD/.yum-clear.conf"
  BUILDVER=$CLRVER
  SWUPD_SERVER_STATE_DIR="$PWD/build/"
elif [ "$BUILDTYPE" = "mix" ]; then
  if [ ! -f "$PWD/.mixer-repopath" ] ; then
    m4 yum.conf.in > "$PWD/.yum-mix.conf"
  else
    repopath=$(cat "$PWD/.mixer-repopath" | xargs realpath)
    m4 -D MIXER_REPO -D MIXER_REPOPATH="$repopath" yum.conf.in > "$PWD/.yum-mix.conf"
  fi
  SWUPD_YUM_CONF="$PWD/.yum-mix.conf"
  BUILDVER=$MIXVER
  SWUPD_SERVER_STATE_DIR="/var/lib/update/"
fi

# if BUILDVER already exists wipe it so it's a fresh build
if [ -d $SWUPD_SERVER_STATE_DIR/image/$BUILDVER ] ; then
  echo -e "Wiping away previous $BUILDVER...\n"
  sudo -E rm -rf "$SWUPD_SERVER_STATE_DIR/www/$BUILDVER"
  sudo -E rm -rf "$SWUPD_SERVER_STATE_DIR/image/$BUILDVER"
fi

# if this is a mix, need to build with the Clear version, but publish the mix version
sudo -E "$BUILDERSCRIPT" -m $BUILDVER $CLRVER

# vi: ts=8 sw=2 sts=2 et tw=80
