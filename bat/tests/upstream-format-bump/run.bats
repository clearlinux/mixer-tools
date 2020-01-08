#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Perform format bump builds to cross upstream format bump" {
  # The kernel-container bundle is deleted in the format bump from 27 to 28
  # and is used to test upstream bundle deletion. Version 29690 is in format
  # 27 and version 29800 is in format 28.
  mixer-init-stripped-down 29690 10
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  # Add bundle deleted by upstream
  mixer-bundle-add kernel-container

  # Create local bundle to be deleted
  create-empty-local-bundle "local-deleted"
  sed -i "s/\(# \[STATUS\]:\).*/\1 Deprecated/" local-bundles/local-deleted
  mixer-bundle-add "local-deleted"

  # Create initial version in the old format
  mixer-build-bundles > $LOGDIR/build_bundles.log

  mixer-build-update > $LOGDIR/build_update.log

  run mixer-versions-update 20 29800
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

  # check deleted upstream/local bundles delete tracking file in +10
  grep -P "20\tkernel-container" update/www/20/Manifest.MoM
  grep -P "20\tlocal-deleted" update/www/20/Manifest.MoM
  grep -P "00000000000\t20\t/usr/share/clear/bundles/kernel-container" update/www/20/Manifest.kernel-container
  grep -P "00000000000\t20\t/usr/share/clear/bundles/local-deleted" update/www/20/Manifest.local-deleted

  # check deleted upstream/local bundle manifest deletions in +20
  grep -v "kernel-container" update/www/30/Manifest.MoM
  grep -v "local-deleted" update/www/30/Manifest.MoM
  test ! -f update/www/30/Manifest.kernel-container
  test ! -f update/www/30/Manifest.local-deleted

  # check deleted bundles no longer tracked by mix
  grep -v "kernel-container" mixbundles
  grep -v "local-deleted" mixbundles

}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
