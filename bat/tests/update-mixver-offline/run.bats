#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Update mixver while offline" {
  mixer-init-stripped-down 25770 10
  export https_proxy=0.0.0.0
  export http_proxy=0.0.0.0
  mixer versions update --mix-version 50
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
