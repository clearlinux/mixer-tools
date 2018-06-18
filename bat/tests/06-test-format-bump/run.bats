#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

# TODO: This test should run 'mixer build upstream-format' instead
# once the tooling required to run custom test docker images for
# is ready.
@test "Perform upstream format bump on last format boundary" {
  upstreamver=$(get-last-format-boundary)
  mixer-init-stripped-down $upstreamver 10

  #Create initial version in the old format
  mixer-build-all > $LOGDIR/build_all.log

  #Update to new upstream in the new format
  mixer-upstream-update $(($upstreamver + 10)) > $LOGDIR/upstream_update.log

  #Build +20
  mixer-build-format-bump-new > $LOGDIR/build_formatbump_new.log

  #Reset upstream for +10 build
  echo -n "$upstreamver" > upstreamversion

  #Build +10
  mixer-build-format-bump-old > $LOGDIR/build_formatbump_old.log

  #restore upstrean
  mv upstreamversion.bump upstreamversion

  #check if LAST_VER is set to +20
  test $(< update/image/LAST_VER) -eq 30

  #check if +20 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/30/Manifest.MoM

  #check if +10 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/30/Manifest.MoM

  #check if builder.conf has the +20 format
  test $(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' builder.conf) -eq 2
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
