#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Perform manual format bump with simplified process" {
  mixer-init-stripped-down $CLRVER 10
  sed -i 's/\(FORMAT\).*/\1 = "1"/' mixer.state

  # create bundle to be deleted
  create-empty-local-bundle "foo"
  # mark as deleted
  sed -i "s/\(# \[STATUS\]:\).*/\1 Deprecated/" local-bundles/foo
  # add foo bundle to mix
  mixer-bundle-add "foo"

  ###############################################################################
  # +0
  #
  # This is the last normal build in the original format. When doing a format
  # bump this build already exists. Building it now because the test needs a
  # normal starting version.
  ###############################################################################

  # build bundles and updates regularly
  mixer-build-bundles > $LOGDIR/build_bundles10.log
  mixer-build-update > $LOGDIR/build_update10.log

  ###############################################################################
  # +10
  #
  # This is the last build in the original format. At this point add ONLY the
  # content relevant to the format bump to the mash to be used. Relevant content
  # should be the only change.
  #
  # mixer will create manifests and update content based on the format it is
  # building for. The format is set in the mixer.state file.
  ###############################################################################

  # update mixer to build version 20, which in our case is the +10
  mixer-mixversion-update 20
  # build bundles normally. At this point the bundles to be deleted should still
  # be part of the mixbundles list and the groups.ini
  mixer-build-bundles > $LOGDIR/build_bundles20.log
  # remove all deleted bundles' content by replacing bundle-info files with empty
  # directories. This causes mixer to fall back to reading content for those
  # bundles from a chroot. The chroots for these bundles will be empty.
  for i in $(grep -lir "\[STATUS\]: Deprecated" local-bundles/); do
    b=$(basename $i)
    sudo rm -f update/image/20/$b-info; sudo mkdir update/image/20/$b
  done
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

  # validate the +10 build
  # MoM is the correct format
  grep -P "MANIFEST\t1" update/www/20/Manifest.MoM
  # bundle to be deleted still exists in the +10
  grep -P "20\tfoo" update/www/20/Manifest.MoM
  # deprecated bundle contains only one deleted file
  grep -P "00000000000\t20\t/usr/share/clear/bundles/foo" update/www/20/Manifest.foo

  ###############################################################################
  # +20
  #
  # This is the first build in the new format. The content is the same as the +10
  # but the manifests might be created differently if a new manifest template is
  # defined for the new format.
  ###############################################################################

  # update mixer to build version 30, which in our case is the +20
  mixer-mixversion-update 30
  # update mixer.state to new format
  sudo sed -i 's/\(FORMAT\).*/\1 = "2"/' mixer.state
  # Fully remove deleted bundles from groups.ini and mixbundles list. This will
  # cause the deprecated bundles to be removed from the MoM entirely. This will
  # not break users who had these bundles because the removed content in the +10
  # caused the bundles to be dropped from client systems at that point.
  for i in $(grep -lir "\[STATUS\]: Deprecated" upstream-bundles/ local-bundles/); do
    b=$(basename $i)
    mixer-bundle-remove $b; sudo sed -i "/\[$b\]/d;/group=$b/d" update/groups.ini;
  done
  # link the +10 bundles to the +20 so we are building the update with the same
  # underlying content. The only things that might change are the manifests
  # (potentially the pack and full-file formats as well, though this is very
  # rare).
  sudo cp -al update/image/20 update/image/30
  # build an update as a minversion, this is the first build where the manifests
  # identify as the new format
  mixer-build-update-minversion 30

  # validate the +20 build
  grep -P "MANIFEST\t2" update/www/30/Manifest.MoM
  grep -v "foo" update/www/30/Manifest.MoM
  test ! -f update/www/30/Manifest.foo
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
