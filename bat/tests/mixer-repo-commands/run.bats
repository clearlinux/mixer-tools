#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Set repo values" {
  mixer-init-stripped-down 25740 10

  # Set value for new key
  run mixer repo set local testkey 2
  val=$(sed -n 's/^testkey.*= //p' .yum-mix.conf)

  [[ "$status" -eq 0 ]]
  [[ "$val" -eq 2 ]]

  # Change value of existing key
  run mixer repo set local testkey 3
  val=$(sed -n 's/^testkey.*= //p' .yum-mix.conf)

  [[ "$status" -eq 0 ]]
  [[ "$val" -eq 3 ]]

  # Attempt to set repo that doesn't exist
  run mixer repo set invalidRepo testkey 3
  [[ "$status" -ne 0 ]]
}

@test "Add repo" {
  mixer-init-stripped-down 25740 10

  # Add new repo
  run mixer repo add newRepo newUrl

  [[ "$status" -eq 0 ]]
  grep -P "^\[newRepo\]" .yum-mix.conf
  grep -P "^name=newRepo" .yum-mix.conf
  grep -P "^baseurl=newUrl" .yum-mix.conf

  # Attempt to add existing repo
  run mixer repo add newRepo newUrl
  [[ "$status" -ne 0 ]]
}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
