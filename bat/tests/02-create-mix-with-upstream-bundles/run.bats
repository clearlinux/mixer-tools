#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create mix 10 with real clear bundle" {
  mixer $MIXARGS init --clear-version $CLRVER --mix-version 10
  
  rm -f $BATS_TEST_DIRNAME/mixbundles                                # Wipe out default bundles
  mixer $MIXARGS bundle add os-core                                  # Add real os-core from upstream
  sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf # Patch default builder.conf
  
  mixer-build-bundles > $LOGDIR/build_bundles.log
  mixer-build-update > $LOGDIR/build_update.log
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
