#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Test that state paths do not exist in the full chroot" {
  upstreamver=$(get-last-format-boundary)
  mixer-init-stripped-down $upstreamver 10

  #Create initial version in the old format
  mixer-build-bundles > $LOGDIR/build_bundles.log

  toRemove="/etc/dnf /var/lib/cache/yum /var/cache/yum /var/cache/dnf /var/lib/dnf /var/lib/rpm"
  for f in $toRemove; do
    test ! -d update/image/10/full/$f
  done
}
