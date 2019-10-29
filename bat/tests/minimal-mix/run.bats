#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup

  # Strip required bundles to minimum package set.
  mixer init --no-default-bundles --all-local
  mixer bundle create no-pkgs
  mixer bundle add no-pkgs
}

@test "Build a minimum mix" {

  run sudo sh -c "sudo mixer build all --native --increment"
  [[ "$status" -eq 0 ]]

  # Should have just one bundle
  bundles=$(grep ^M\\. update/www/10/Manifest.MoM|wc -l)
  [ "$bundles" -eq "1" ]

  # Manifest no-pkgs from version 10 should be listed
  grep "^M\..*	10	no-pkgs$" update/www/10/Manifest.MoM

  # Build an update
  run sudo sh -c "sudo mixer build all --native --increment"
  [[ "$status" -eq 0 ]]

  bundles=$(grep ^M\\. update/www/20/Manifest.MoM|wc -l)
  [ "$bundles" -eq "1" ]

  # Manifest no-pkgs from version 10 should be listed
  grep "^M\..*	10	no-pkgs$" update/www/20/Manifest.MoM

}
