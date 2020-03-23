#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create mix 10 with content chroot" {
  customDir="$BATS_TEST_DIRNAME"/custom-content
  fullChroot=update/image/10/full

  # Create custom content chroot with directories, a file, and symlinks.
  # Non-root ownership and non-default permissions are also tested.
  mkdir -p "$customDir"/usr/bin
  sudo chown root:root "$customDir"/usr
  sudo chown root:root "$customDir"/usr/bin
  mkdir -m 777  "$customDir"/dirPerm
  sudo chown 1000:1000 "$customDir"/dirPerm
  sudo touch "$customDir"/usr/bin/foo
  sudo chmod 777 "$customDir"/usr/bin/foo
  sudo chown 1000:1000 "$customDir"/usr/bin/foo
  ln -s usr "$customDir"/dirLink
  sudo chown -h 1000:1000 "$customDir"/dirLink
  ln -s usr/bin/foo "$customDir"/fileLink
  sudo chown -h 1000:1000 "$customDir"/fileLink

  # Create a file/dir with setuid, setgid, and sticky perms
  touch "$customDir"/specialPermsFile
  chmod 7777 "$customDir"/specialPermsFile
  mkdir "$customDir"/specialPermsDir
  chmod 7777 "$customDir"/specialPermsDir

  mixer-init-stripped-down "$CLRVER" 10

  # Create local bundle definition with a package and content chroot
  create-empty-local-bundle "bundle1"
  add-package-to-local-bundle "bsdiff" "bundle1"
  echo "content(custom-content)" >> "$LOCAL_BUNDLE_DIR"/bundle1
  mixer-bundle-add "bundle1"

  # Create bundle with identical content chroot
  create-empty-local-bundle "bundle2"
  echo "content(custom-content)" >> "$LOCAL_BUNDLE_DIR"/bundle2
  mixer-bundle-add "bundle2"

  mixer-build-bundles > "$LOGDIR"/build_bundles.log
  mixer-build-update > "$LOGDIR"/build_update.log

  # Check symlinks match
  fileLink1=$(readlink $customDir/fileLink)
  fileLink2=$(readlink $fullChroot/fileLink)
  [[ "$fileLink1" = "$fileLink2" ]]

  dirLink1=$(readlink $customDir/dirLink)
  dirLink2=$(readlink $fullChroot/dirLink)
  [[ "$dirLink1" = "$dirLink2" ]]

  # Check expected permissions and ownership copied to full chroot
  statList1=("$customDir/fileLink" "$customDir/dirLink" "$customDir/usr/bin/foo"
    "$customDir/dirPerm" "$customDir/specialPermsFile" "$customDir/specialPermsDir")
  statList2=("$fullChroot/fileLink" "$fullChroot/dirLink" "$fullChroot/usr/bin/foo"
    "$fullChroot/dirPerm" "$fullChroot/specialPermsFile" "$fullChroot/specialPermsDir")

  for ((i=0;i<${#statList1[@]};i++)); do
    filePerms1=$(stat -c '%A:%U:%G' "${statList1[i]}")
    filePerms2=$(stat -c '%A:%U:%G' "${statList2[i]}")
    [[ "$filePerms1" = "$filePerms2" ]]
  done

  # Verify that manifest contains the content chroot files and the
  # bsdiff executable
  grep -P "\t10\t/usr/bin/foo" update/www/10/Manifest.bundle1
  grep -P "\t10\t/dirPerm" update/www/10/Manifest.bundle1
  grep -P "\t10\t/specialPermsFile" update/www/10/Manifest.bundle1
  grep -P "\t10\t/specialPermsDir" update/www/10/Manifest.bundle1
  grep -P "\t10\t/usr/bin/bsdiff" update/www/10/Manifest.bundle1
  grep -P "L...\t.*\t10\t/fileLink" update/www/10/Manifest.bundle1
  grep -P "L...\t.*\t10\t/dirLink" update/www/10/Manifest.bundle1

  grep -P "\t10\t/usr/bin/foo" update/www/10/Manifest.bundle2
  grep -P "\t10\t/dirPerm" update/www/10/Manifest.bundle2
  grep -P "\t10\t/specialPermsFile" update/www/10/Manifest.bundle1
  grep -P "\t10\t/specialPermsDir" update/www/10/Manifest.bundle1
  grep -P "L...\t.*\t10\t/fileLink" update/www/10/Manifest.bundle2
  grep -P "L...\t.*\t10\t/dirLink" update/www/10/Manifest.bundle2
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
