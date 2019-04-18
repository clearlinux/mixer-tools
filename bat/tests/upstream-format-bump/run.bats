#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Perform format bump builds to cross upstream format bump" {
  mixer-init-stripped-down 25740 10
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  # Create initial version in the old format
  mixer-build-bundles > $LOGDIR/build_bundles.log

  mixer-build-update > $LOGDIR/build_update.log

  run mixer-versions-update 20 26000
  # Updating versions should have output this as part of the error
  [[ "$output" =~ "mixer build upstream-format" ]]

  mixer-build-upstream-format-bump $((format+1))

  #check if +10 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/20/Manifest.MoM

  # #check if +20 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/30/Manifest.MoM

  #check if +10 format is set to formatN+1
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/20/Manifest.MoM

  format=$((format+1))
  #check if +20 format is set to formatN
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/30/Manifest.MoM

  #check LAST_VER and PREVIOUS_MIX_VERSION match
  test $(< update/image/LAST_VER) -eq 30
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 30

}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
