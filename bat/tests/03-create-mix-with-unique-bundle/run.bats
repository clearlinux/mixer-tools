#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  setup_builder_conf
}

@test "Create initial mix 10" {
  mixer-init-versions $CLRVER 10
  clean-bundle-dir
  mixer-build-bundles
  mixer-create-update
}

@test "Create version 20 with unique custom bundle" {
  localize_builder_conf
  mixer-init-versions $CLRVER 20
  download-rpm "http://rpmfind.net/linux/fedora/linux/development/rawhide/Everything/x86_64/os/Packages/j/json-c-0.13.1-1.fc29.i686.rpm"
  mixer-add-rpms
  add-bundle "testbundle"
  add-package "json-c" "testbundle"
  mixer-build-bundles
  mixer-create-update > $BATS_TEST_DIRNAME/create_update-20.log
}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
