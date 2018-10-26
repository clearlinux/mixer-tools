#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create stripped down mix 10 with custom content in custom bundle" {
  mixer-init-stripped-down $CLRVER 10
  localize_builder_conf

  download-rpm json-c
  mixer-add-rpms
  create-empty-local-bundle "testbundle"
  add-package-to-local-bundle "json-c" "testbundle"
  mixer-bundle-add "testbundle"

  mixer-build-bundles > $LOGDIR/build_bundles.log
  mixer-build-update > $LOGDIR/build_update.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
