#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "build delta-packs after build all" {
  current_ver=$(get-current-version)
  mixer-init-stripped-down "$current_ver" 10
  sudo -E mixer build all --native --increment
  mixer-build-all
  sudo -E mixer build delta-packs --native --previous-versions=1

  test $(< mixversion) -eq 20
  test -e update/www/20/pack-os-core-from-10.tar
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
