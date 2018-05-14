#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create stripped down mix 10 with blended bundles" {
  mixer-init-stripped-down $CLRVER 10
  localize_builder_conf

  download-rpm "http://rpmfind.net/linux/fedora/linux/development/rawhide/Everything/x86_64/os/Packages/j/json-c-0.13.1-2.fc29.x86_64.rpm"
  mixer-add-rpms

  # Put custom content in upstream bundle
  create-empty-local-bundle "os-core-update"
  add-package-to-local-bundle "bsdiff" "os-core-update"
  add-package-to-local-bundle "json-c" "os-core-update"
  mixer-bundle-add "os-core-update"

  # Put upstream content in custom bundle
  create-empty-local-bundle "testbundle"
  add-package-to-local-bundle "json-c" "testbundle"
  add-package-to-local-bundle "bsdiff" "testbundle"
  mixer-bundle-add "testbundle"

  mixer-build-bundles > $LOGDIR/build_bundles.log
  mixer-build-update > $LOGDIR/build_update.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
