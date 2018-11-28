#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Verify clear repo is unable to be removed by mixin" {
  run sudo mixin repo remove clear

  [[ "$status" -eq 1 ]]
  [[ "$output" =~ "The clear repo is mandatory and cannot be removed" ]]
  [[ ! "$output" =~ "Removed clear repo." ]]
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
