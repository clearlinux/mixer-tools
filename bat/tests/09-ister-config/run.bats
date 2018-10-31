#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Build image using default ister config" {
  mixer $MIXARGS init --clear-version $CLRVER --mix-version 10
  mixer $MIXARGS bundle add editors

  # `build image` should fail because the prerequisite commands are not executed.
  # This is intentional in order to reduce test execution time.
  # The goal is to unit test the creation of ister config file with relevant bundles.
  run sudo mixer $MIXARGS build image --native
  [[ "$status" -eq 1 ]]
  [[ "$output" =~ "release-image-config.json not found" ]]
  [[ "$output" =~ "Copying image template" ]]
  [[ "$output" =~ "Updating image bundle list based on mixbundles" ]]
}

@test "Update ister config file with mix bundle list" {
  run cat release-image-config.json
  [[ "$status" -eq 0 ]]
  [[ "$output" =~ \"os-core\" ]]
  [[ "$output" =~ \"os-core-update\" ]]
  [[ "$output" =~ \"kernel-native\" ]]
  [[ "$output" =~ \"bootloader\" ]]
  [[ "$output" =~ \"editors\" ]]
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
