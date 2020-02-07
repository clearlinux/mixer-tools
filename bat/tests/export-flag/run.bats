#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create mix 10 with un-export flag" {

  mixer-init-stripped-down "$CLRVER" 10

  # Create local bundle definition with a package and un-export file path
  create-empty-local-bundle "bundle1"
  add-package-to-local-bundle "bsdiff" "bundle1"
  mixer-bundle-add "bundle1"

  mixer-build-bundles > "$LOGDIR"/build_bundles.log
  mixer-build-update > "$LOGDIR"/build_update.log

  # Verify that manifest contains the export flags for the bsdiff files
  grep -P "F..x\t[A-Za-z0-9]*\t10\t/usr/bin/bsdiff" update/www/10/Manifest.bundle1 # should have export flag
  grep -P "F..x\t[A-Za-z0-9]*\t10\t/usr/bin/bspatch" update/www/10/Manifest.bundle1 # should have export flag

  mixer versions update
  echo "un-export(/usr/bin/bsdiff)" >> "$LOCAL_BUNDLE_DIR"/bundle1
  mixer-build-bundles > "$LOGDIR"/build_bundles.log
  mixer-build-update > "$LOGDIR"/build_update.log
  grep -P "F..\.\t[A-Za-z0-9]*\t20\t/usr/bin/bsdiff" update/www/20/Manifest.bundle1 # should not have export flag
  grep -P "F..x\t[A-Za-z0-9]*\t10\t/usr/bin/bspatch" update/www/20/Manifest.bundle1 # should have export flag

}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
