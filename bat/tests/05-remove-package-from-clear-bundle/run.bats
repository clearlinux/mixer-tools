#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  setup_builder_conf
}

@test "Create initial mix 10" {
  mixer-init-versions $CLRVER 10
  clean-bundle-dir
  mixer-build-chroots
  mixer-create-update
}

@test "Create version 20 with Clear editors bundle added" {
  mixer-init-versions $CLRVER 20
  add-bundle "editors"
  add-package "joe" "editors"
  add-package "nano" "editors"
  mixer-build-chroots
  mixer-create-update > $BATS_TEST_DIRNAME/create_update-20.log
}

@test "Create version 30 with nano removed from editors bundle" {
  mixer-init-versions $CLRVER 30
  remove-package "nano" "editors"
  mixer-build-chroots
  mixer-create-update > $BATS_TEST_DIRNAME/create_update-30.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
