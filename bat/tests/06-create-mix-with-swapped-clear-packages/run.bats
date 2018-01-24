#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  setup_builder_conf
}

@test "Create initial mix 10" {
  mixer-init-versions $CLRVER 10
  clean-bundle-dir
  add-bundle "os-core-update"
  add-package "swupd-client" "os-core-update"
  add-package "bsdiff" "os-core-update"
  mixer-build-chroots
  mixer-create-update
}

@test "Create version 20 with swupd moved from os-core-update into os-core" {
  mixer-init-versions $CLRVER 20
  remove-package "swupd-client" "os-core-update"
  add-package "swupd-client" "os-core"
  mixer-build-chroots
  mixer-create-update > $BATS_TEST_DIRNAME/create_update-20.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
