#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create mix 10 with content chroot" {
  customDir="$BATS_TEST_DIRNAME"/custom-content
  fullChroot=update/image/10/full

  # Create custom content chroot with directories, a file, and symlinks
  mkdir -p "$customDir"/usr/bin
  mkdir -m 744  "$customDir"/dirPerm
  touch "$customDir"/usr/bin/foo
  chmod 744 "$customDir"/usr/bin/foo
  ln -s usr "$customDir"/dirLink
  ln -s usr/bin/foo "$customDir"/fileLink

  mixer-init-stripped-down "$CLRVER" 10

  # Create local bundle definition with a package and content chroot
  create-empty-local-bundle "bundle1"
  add-package-to-local-bundle "bsdiff" "bundle1"
  echo "content(custom-content)" >> "$LOCAL_BUNDLE_DIR"/bundle1
  mixer-bundle-add "bundle1"

  mixer-build-bundles > "$LOGDIR"/build_bundles.log
  mixer-build-update > "$LOGDIR"/build_update.log

  # Check symlinks match
  fileLink1=$(readlink $customDir/fileLink)
  fileLink2=$(readlink $fullChroot/fileLink)
  [[ "$fileLink1" = "$fileLink2" ]]

  dirLink1=$(readlink $customDir/dirLink)
  dirLink2=$(readlink $fullChroot/dirLink)
  [[ "$dirLink1" = "$dirLink2" ]]

  # Check expected permissions copied to full chroot
  filePerm1=$(stat -c '%A' $customDir/usr/bin/foo)
  filePerm2=$(stat -c '%A' $fullChroot/usr/bin/foo)
  [[ "$filePerm1" = "$filePerm2" ]]

  dirPerm1=$(stat -c '%A' $customDir/dirPerm)
  dirPerm2=$(stat -c '%A' $fullChroot/dirPerm)
  [[ "$dirPerm1" = "$dirPerm2" ]]

  # Verify that manifest contains the content chroot files and the
  # bsdiff executable
  grep -P "\t10\t/usr/bin/foo" update/www/10/Manifest.bundle1
  grep -P "\t10\t/dirPerm" update/www/10/Manifest.bundle1
  grep -P "\t10\t/usr/bin/bsdiff" update/www/10/Manifest.bundle1
  grep -P "L...\t.*\t10\t/fileLink" update/www/10/Manifest.bundle1
  grep -P "L...\t.*\t10\t/dirLink" update/www/10/Manifest.bundle1
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
