#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup

  mixer-init-stripped-down $CLRVER 10
  cp mixer.state current.state
}

function copy_test_state() {
  cp $1 mixer.state

  # Update test state file to current format
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' current.state)
  sed -i "s/\(FORMAT\).*/\1 = \"$format\"/" mixer.state
}

@test "Convert all state versions to latest with version update" {
   # Verify that all state version conversions work
  for f in "configs"/*
  do
    copy_test_state $f

    mixer versions --offline

    diff mixer.state current.state

    mixer-init-stripped-down $CLRVER 10
  done
}

@test "Convert to latest state with build all" {
  # When LAST_VER exists, PREVIOUS_MIX_VERSION is set to LAST_VER during
  # conversions. Otherwise, PREVIOUS_MIX_VERSION is set to 0.

  copy_test_state configs/1.0_mixer.state

  # Verify that state is converted when LAST_VER doesn't exist
  sudo mixer build all --native
  diff mixer.state current.state

  # Increment versions and update expected PREVIOUS_MIX_VERSION value
  mixer versions update --mix-version 20
  copy_test_state configs/1.0_mixer.state
  sed -i 's/\(PREVIOUS_MIX_VERSION\).*/\1 = "10"/' current.state

  # Verify that state is converted and initialized to LAST_VER. At the
  # time of the PREVIOUS_MIX_VERSION initialization, LAST_VER should
  # still be 10.
  sudo mixer build all --native
  diff mixer.state current.state
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
