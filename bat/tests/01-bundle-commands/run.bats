#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Initialize a mix at version 10" {
  mixer $MIXARGS init --clear-version $CLRVER --mix-version 10
  [[ -f $BATS_TEST_DIRNAME/builder.conf ]]
  [[ -f $BATS_TEST_DIRNAME/mixbundles ]]
  [[ $(wc -l < $BATS_TEST_DIRNAME/mixbundles) == 4 ]]
  [[ -d $BATS_TEST_DIRNAME/local-bundles ]]
  [[ -d $BATS_TEST_DIRNAME/upstream-bundles/clr-bundles-$CLRVER/bundles ]]
}

@test "List the bundles in the mix" {
  run mixer $MIXARGS bundle list
  [[ ${#lines[@]} -eq 4 ]]              # Exactly 4 bundles in the mix
  [[ "$output" =~ os-core[[:space:]] ]] # To avoid just matching os-core-update
  [[ "$output" =~ os-core-update ]]
  [[ "$output" =~ kernel-native ]]
  [[ "$output" =~ bootloader ]]
}

@test "Add an upstream bundle to the mix" {
  mixer $MIXARGS bundle add editors

  run mixer $MIXARGS bundle list
  [[ ${#lines[@]} -gt 4 ]]                         # More bundles in list now
  [[ "$output" =~ editors[[:space:]]+\(upstream ]] # "editors" bundle is from upstream
}

@test "Edit upstream bundle" {
  run mixer $MIXARGS bundle list
  [[ "$output" =~ editors[[:space:]]+\(upstream ]]              # "editors" bundle is from upstream
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles | wc -l) == 0 ]] # Nothing in local-bundles

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 0 ]]                                      # 'list local' returns no results

  mixer $MIXARGS bundle edit editors --suppress-editor
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles) = "editors" ]]  # local-bundles only has "editors"

  run mixer $MIXARGS bundle list
  [[ "$output" =~ editors[[:space:]]+\(local ]]                 # "editors" bundle is now from local

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 1 ]]                                      # 'list local' returns 1 result
  [[ "$output" =~ editors.*masking ]]                           # "editors" bundle is masking upstream
}

@test "Create original bundle and add to mix" {
  mixer $MIXARGS bundle edit foobar --suppress-editor --add

  run ls -1q $BATS_TEST_DIRNAME/local-bundles
  [[ ${#lines[@]} -eq 2 ]]                     # 2 bundles in local-bundles
  [[ "$output" =~ foobar ]]                    # local-bundles now contains "foobar"

  run mixer $MIXARGS bundle list
  [[ "$output" =~ foobar[[:space:]]+\(local ]] # "foobar" bundle is from local

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 2 ]]                     # 'list local' returns 2 results
  [[ "$output" =~ .*foobar.* ]]                # "foobar" bundle is in output
}

@test "Remove bundle from mix" {
  mixer $MIXARGS bundle remove editors

  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles | wc -l) == 2 ]] # Still 2 bundles in local-bundles

  ! mixer $MIXARGS bundle list | grep -q editors                # "editors" no longer in mix
}

@test "Validate a bundle" {
  echo "package" >> $BATS_TEST_DIRNAME/local-bundles/foobar

  mixer $MIXARGS bundle validate foobar

  ! mixer $MIXARGS bundle validate foobar --strict
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
