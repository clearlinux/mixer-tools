#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "build delta-packs after build all for different versions" {
  current_ver=$(get-current-version)
  mixer-init-stripped-down "$current_ver" 10
  mixer-build-all
  sudo mixer versions update
  mixer-build-all
  mixer-build-delta-packs 10
  sudo rm -rf ./update/image/10/full/usr/lib/os-release
  sudo mixer versions update
  mixer-build-all
  mixer-build-delta-packs 10
  test $(ls ./update/www/30/delta/ | wc -l) -eq 1
  test $(ls ./update/www/10/delta/ | wc -l) -eq 0
  test $(ls ./update/www/20/delta/ | wc -l) -eq 1
  }
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
