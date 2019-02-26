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
  mixer bundle create foo.bar           # 'bundle list' should work even if an invalid bundle is created

  bundles=$(mixer $MIXARGS bundle list | grep -v "(included)")
  [[ $(echo "$bundles" | wc -l) -eq 4 ]] # Exactly 4 bundles in the mix
  [[ "$bundles" =~ os-core[[:space:]] ]] # To avoid just matching os-core-update
  [[ "$bundles" =~ os-core-update ]]
  [[ "$bundles" =~ kernel-native ]]
  [[ "$bundles" =~ bootloader ]]

  rm -f local-bundles/foo.bar           # Delete invalid bundle from local-bundles (test case clean up)
}

@test "Add an upstream bundle to the mix" {
  mixer $MIXARGS bundle add editors

  bundles=$(mixer $MIXARGS bundle list | grep -v "(included)")
  [[ $(echo "$bundles" | wc -l) -gt 4 ]]  # More bundles in list now
  [[ "$bundles" =~ editors[[:space:]]+\(upstream ]] # "editors" bundle is from upstream
}

@test "Create upstream bundle" {
  run mixer $MIXARGS bundle list
  [[ "$output" =~ editors[[:space:]]+\(upstream ]]              # "editors" bundle is from upstream
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles | wc -l) == 0 ]] # Nothing in local-bundles

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 0 ]]                                      # 'list local' returns no results

  mixer $MIXARGS bundle create editors
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles) = "editors" ]]  # local-bundles only has "editors"

  run mixer $MIXARGS bundle list
  [[ "$output" =~ editors[[:space:]]+\(local ]]                 # "editors" bundle is now from local

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 1 ]]                                      # 'list local' returns 1 result
  [[ "$output" =~ editors.*masking ]]                           # "editors" bundle is masking upstream
}

@test "Remove bundle from mix" {
  mixer $MIXARGS bundle remove editors

  ! mixer $MIXARGS bundle list | grep -q editors                # "editors" bundle is no longer in mix

  # "editors" bundle is still in local-bundles
  mixer $MIXARGS bundle list local | grep -q editors
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles | wc -l) == 1 ]]
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles) = "editors" ]]

  mixer $MIXARGS bundle remove editors --local

  # "editors" bundle is no more in local-bundles
  ! mixer $MIXARGS bundle list local | grep -q editors
  [[ $(ls -1q $BATS_TEST_DIRNAME/local-bundles | wc -l) == 0 ]]
}

@test "Create original bundle and add to mix" {
  mixer $MIXARGS bundle create foobar --add
  mixer $MIXARGS bundle edit foocar --suppress-editor --add     # Create and add bundle using `edit` command

  run ls -1q $BATS_TEST_DIRNAME/local-bundles
  [[ ${#lines[@]} -eq 2 ]]                     # 2 bundles in local-bundles
  [[ "$output" =~ foobar ]]                    # local-bundles now contains "foobar"
  [[ "$output" =~ foocar ]]                    # local-bundles now contains "foocar"

  run mixer $MIXARGS bundle list local
  [[ ${#lines[@]} -eq 2 ]]                     # 'list local' returns 2 results
  [[ "$output" =~ .*foobar.* ]]                # "foobar" bundle is in output
  [[ "$output" =~ .*foocar.* ]]                # "foocar" bundle is in output

  run mixer $MIXARGS bundle list
  [[ "$output" =~ foobar[[:space:]]+\(local ]] # "foobar" bundle is from local
  [[ "$output" =~ foocar[[:space:]]+\(local ]] # "foocar" bundle is from local

  # Delete bundle from local-bundles and mix (test case clean up)
  mixer $MIXARGS bundle remove foocar
  rm -f local-bundles/foocar
}

@test "Skip invalid bundles from mix" {
  mixer $MIXARGS bundle create foocar foo.car foojar --add      # Create and add valid as well as invalid bundles

  mixer $MIXARGS bundle list | grep -q foocar     # "foocar" bundle is in the mix
  mixer $MIXARGS bundle list | grep -q foojar     # "foojar" bundle is in the mix

  run $(mixer $MIXARGS bundle list | grep foo.car)              # "foo.car" is an invalid bundle and is not in the mix
  [[ ${#lines[@]} -eq 0 ]]

  # Delete bundle from local-bundles and mix (test case clean up)
  mixer $MIXARGS bundle remove foocar foojar
  rm -f local-bundles/foocar local-bundles/foojar local-bundles/foo.car
}

@test "Validate a bundle" {
  echo "package" >> $BATS_TEST_DIRNAME/local-bundles/foobar
  mixer $MIXARGS bundle create foo.bar

  run mixer $MIXARGS bundle validate foobar
  [[ "$status" -eq 0 ]]                        # basic validation should pass

  run mixer $MIXARGS bundle validate foobar --strict
  [[ "$status" -eq 1 ]]                        # strict validation should fail
  [[ "$output" =~ "Empty Description in bundle header" ]]
  [[ "$output" =~ "Empty Maintainer in bundle header" ]]
  [[ "$output" =~ "Empty Status in bundle header" ]]
  [[ "$output" =~ "Empty Capabilities in bundle header" ]]

  run mixer $MIXARGS bundle validate foo.bar
  [[ "$status" -eq 1 ]]                        # basic validation should fail
  [[ "$output" =~ "Invalid bundle name \"foo.bar\" derived from file" ]]

  run mixer $MIXARGS bundle validate foo.bar --strict
  [[ "$status" -eq 1 ]]                        # strict validation should fail
  [[ "$output" =~ "Invalid bundle name \"foo.bar\" derived from file" ]]
  [[ "$output" =~ "Invalid bundle name \"foo.bar\" in bundle header Title" ]]
  [[ "$output" =~ "Empty Description in bundle header" ]]
  [[ "$output" =~ "Empty Maintainer in bundle header" ]]
  [[ "$output" =~ "Empty Status in bundle header" ]]
  [[ "$output" =~ "Empty Capabilities in bundle header" ]]

  rm -f local-bundles/foo.bar           # Delete invalid bundle from local-bundles (test case clean up)
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
