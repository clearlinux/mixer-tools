#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

# TODO: just call the auto-format-bump capability when available. Not using the old
# auto-format-bump functionality because it will be changing very soon.

@test "Attempt to create delta packs over format bump" {
  mixer-init-stripped-down $CLRVER 10
  sed -i 's/\(FORMAT\).*/\1 = "1"/' mixer.state

  ###############################################################################
  # +0
  ###############################################################################

  # build bundles and updates regularly
  mixer-build-bundles > $LOGDIR/build_bundles10.log
  mixer-build-update > $LOGDIR/build_update10.log

  ###############################################################################
  # +10
  ###############################################################################

  # update mixer to build version 20, which in our case is the +10
  mixer-mixversion-update 20
  # build bundles normally. At this point the bundles to be deleted should still
  # be part of the mixbundles list and the groups.ini
  mixer-build-bundles > $LOGDIR/build_bundles20.log
  # no deleted bundles

  # Replace the +10 version in /usr/lib/os-release with +20 version and write the
  # new format to the format file on disk.  This is so clients will already be on
  # the new format when they update to the +10 because the content is the same as
  # the +20.
  sudo sed -i 's/\(VERSION_ID=\).*/\130/' update/image/20/full/usr/lib/os-release
  echo 2 | sudo tee update/image/20/full/usr/share/defaults/swupd/format
  # build update based on the modified bundle information. This is *not* a
  # minversion and these manifests must be built with the mixer from the original
  # format (if manifest format changes).
  mixer-build-update > $LOGDIR/build_update20.log

  # not validating the bump itself, we have another test for that

  ###############################################################################
  # +20
  ###############################################################################

  # update mixer to build version 30, which in our case is the +20
  mixer-mixversion-update 30
  # update mixer.state to new format
  sudo sed -i 's/\(FORMAT\).*/\1 = "2"/' mixer.state
  # no deleted bundles

  # link the +10 bundles to the +20 so we are building the update with the same
  # underlying content. The only things that might change are the manifests
  # (potentially the pack and full-file formats as well, though this is very
  # rare).
  sudo cp -al update/image/20 update/image/30
  # build an update as a minversion, this is the first build where the manifests
  # identify as the new format
  mixer-build-update-minversion 30

  # not validating the bump itself, we have another test for that

  ###############################################################################
  # try to create delta packs for the previous two versions
  ###############################################################################

  # wipe old image dirs so we make sure we can't create it, then make sure we
  # "Found 0 previous versions" and that we were "skipping delta-pack creation
  # over format bump" when building to make sure we didn't even try.
  sudo rm -rf image/www/{10,20}
  # redirect both stderr and stdout so we can check for the warning as well
  mixer-build-delta-packs 2 &> $LOGDIR/build_delta-packs30.log
  grep "Found 0 previous versions" $LOGDIR/build_delta-packs30.log
  grep "skipping delta-pack creation over format bump" $LOGDIR/build_delta-packs30.log
  # check mixer didn't create them anyways
  [ ! -f update/www/30/pack-os-core-from-10.tar ]
  [ ! -f update/www/30/pack-os-core-from-20.tar ]
  [ ! -f update/www/30/pack-os-core-update-index-from-10.tar ]
  [ ! -f update/www/30/pack-os-core-update-index-from-20.tar ]
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
