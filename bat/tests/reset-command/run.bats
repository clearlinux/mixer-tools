#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "reset" {
  mixer-init-stripped-down latest 10
  sudo mixer build all --format 1 --native
  mixer-versions-update 20 latest
  sudo mixer build all --format 2 --native
  mixer-versions-update 30 latest
  sudo mixer build all --format 3 --native
  #check LAST_VER and PREVIOUS_MIX_VERSION match
  test $(< update/image/LAST_VER) -eq 30
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 20
  sudo mixer reset --to 20
  #check LAST_VER and PREVIOUS_MIX_VERSION match
  test $(< update/image/LAST_VER) -eq 20
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 10
  test -d "./update/www/30"
  test -d "./update/image/30"
  sudo mixer reset --to 10 --clean
  #check LAST_VER and PREVIOUS_MIX_VERSION match
  test $(< update/image/LAST_VER) -eq 10
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 0
  test ! -d "./update/www/20"
  test ! -d "./update/image/30"
  }
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80

