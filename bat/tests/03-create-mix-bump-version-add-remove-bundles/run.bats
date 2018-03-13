#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create initial stripped down mix 10" {
  mixer-init-stripped-down $CLRVER 10
  mixer-build-bundles > $LOGDIR/build_bundles_10.log
  mixer-build-update > $LOGDIR/build_update_10.log
}

@test "Create version 20 with more stripped down upstream bundles" {
  mixer-versions-update 20
  
  create-empty-local-bundle "editors"
  add-package-to-local-bundle "joe" "editors"
  mixer-bundle-add "editors"
  
  create-empty-local-bundle "os-core-update"
  add-package-to-local-bundle "bsdiff" "os-core-update"
  mixer-bundle-add "os-core-update"
  
  mixer-build-bundles > $LOGDIR/build_bundles_20.log
  mixer-build-update > $LOGDIR/build_update_20.log
}

@test "Create version 30 with an upstream bundle deleted" {
  mixer-versions-update 30
  mixer-bundle-remove "editors"
  mixer-build-bundles > $LOGDIR/build_bundles_30.log
  mixer-build-update > $LOGDIR/build_update_30.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
