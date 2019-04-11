#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Perform build with --clean flag" {
  mixer-init-stripped-down 25740 10
  mixer-build-all
  mixer-mixversion-update 20
  mixer-build-all

  # Rebuild v20 with --clean
  run sudo mixer build all --clean --native
  [[ $status -eq 0 ]]

  mom="$BATS_TEST_DIRNAME"/update/www/20/Manifest.MoM

  test $(sed -n 's/^version:\t//p' "$mom") -eq 20
  test $(sed -n 's/^previous:\t//p' "$mom") -eq 10
  test $(< "$BATS_TEST_DIRNAME"/update/image/LAST_VER) -eq 20

  # Check that PREVIOUS_MIX_VERSION in mixer.state is correct
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 10

  # Erase PREVIOUS_MIX_VERSION. This will cause it to default to
  # LAST_VER which will lead to an incorrectly generated manifest.
  sed -i "/PREVIOUS_MIX_VERSION/d" mixer.state

  run sudo mixer build all --clean --native
  [[ $status -eq 0 ]]

  # Verify that previous is incorrect
  test $(sed -n 's/^previous:\t//p' "$mom") -ne 10
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
