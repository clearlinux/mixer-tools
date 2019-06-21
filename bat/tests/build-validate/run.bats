#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup

  # The update directory is created once and copied to update-backup which
  # is reused for each test to avoid regenerating update multiple times.
  if [ -d "${BATS_TEST_DIRNAME}/update-backup" ]; then
    sudo rm -rf "${BATS_TEST_DIRNAME}"/update
    sudo cp -r "${BATS_TEST_DIRNAME}"/update-backup "${BATS_TEST_DIRNAME}"/update
    return
  fi

  upstreamver=$(get-last-format-boundary)

  # Strip required bundles to minimum package set.
  mixer init --no-default-bundles
  mixer bundle create no-pkgs
  mixer bundle add os-core os-core-update no-pkgs
  echo "filesystem" > $LOCAL_BUNDLE_DIR/os-core
  echo "clr-bundles" > $LOCAL_BUNDLE_DIR/os-core-update

  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  # testbundle1 will be modified 
  mixer bundle create testbundle1
  add-package-to-local-bundle "bsdiff" "testbundle1"
  mixer bundle add testbundle1

  sudo mixer build all --increment --native

  # Modify testbundle1
  add-package-to-local-bundle "helloworld" "testbundle1"

  # testbundle2 is an added bundle
  mixer bundle create testbundle2
  add-package-to-local-bundle "bsdiff" "testbundle2"
  mixer bundle add testbundle2

  sudo mixer build all --increment --native

  # Create format bump
  sudo mixer build format-bump --new-format $((format+1)) --native

  # Store a copy of update so that it doesn't need to be regenerated
  sudo cp -r "${BATS_TEST_DIRNAME}"/update "${BATS_TEST_DIRNAME}"/update-backup
}

@test "MCA compare manifests with added, modified, and unchanged bundles" {
  run sudo sh -c "mixer build validate --from 10 --to 20 --native"

  [[ "$status" -eq 0 ]]

  [[ "$output" =~ "** Summary: No errors detected in manifests" ]]
  [[ "$output" =~ "Added bundles: 1" ]]
  [[ "$output" =~ "Changed bundles: 2" ]]
  [[ "$output" =~ "Deleted bundles: 0" ]]

  # testbundle2 is added. testbundle1 and os-core are modified
  [[ "$output" =~ "| testbundle1" ]]
  [[ "$output" =~ "| testbundle2" ]]
  [[ "$output" =~ "| os-core" ]]
}

@test "MCA compare to +10" {
  run sudo mixer build validate --from 20 --to 30 --native

  [[ "$status" -eq 0 ]]

  [[ "$output" =~ "** Summary: No errors detected in manifests" ]]
  [[ "$output" =~ "Added bundles: 0" ]]
  [[ "$output" =~ "Changed bundles: 2" ]]
  [[ "$output" =~ "Deleted bundles: 0" ]]

  # When updating to the +10, the special case files in os-core and
  # os-core-update should be modified
  [[ "$output" =~ "| os-core" ]]
  [[ "$output" =~ "| os-core-update" ]]
}

@test "MCA compare +10 to +20" {
  run sudo mixer build validate --from 30 --to 40 --native

  [[ $status -eq 0 ]]

  # In a +10 to +20 update, no bundles are changed and a minversion
  # is performed
  [[ $output =~ "WARNING: If this is not a +10 to +20 comparison, expected file changes are missing from os-core/os-core-update" ]]
  [[ $output =~ "** Summary: No errors detected in manifests" ]]
  [[ $output =~ "Added bundles: 0" ]]
  [[ $output =~ "Changed bundles: 0" ]]
  [[ $output =~ "Deleted bundles: 0" ]]
  [[ $output =~ "** Minversion bump detected" ]]
}

@test "MCA standard comparison error handling" {
  hash1=d93a5e9129361e28b9e244fe422234e3a1794b001a082aeb78e16fd881673a2c
  hash2=6c23df6efcd6fc401ff1bc67c970b83eef115f6473db4fb9d57e5de317eba96e

  # Insert fake added, deleted, and modified files
  sudo sh -c "echo -e 'F...\t$hash1\t10\t/fakeDel' >> ${BATS_TEST_DIRNAME}/update/www/10/Manifest.os-core"
  sudo sh -c "echo -e 'F...\t$hash1\t20\t/fakeAdd' >> ${BATS_TEST_DIRNAME}/update/www/20/Manifest.os-core"
  sudo sh -c "echo -e 'F...\t$hash1\t10\t/fakeMod' >> ${BATS_TEST_DIRNAME}/update/www/10/Manifest.os-core"
  sudo sh -c "echo -e 'F...\t$hash2\t20\t/fakeMod' >> ${BATS_TEST_DIRNAME}/update/www/20/Manifest.os-core"

  # Prevent mandatory file changes in os-core to create special case errors
  releaseHash=$(awk '/os-release$/ {print $2}' "${BATS_TEST_DIRNAME}"/update/www/10/Manifest.os-core)
  versionHash=$(awk '/version$/ {print $2}' ${BATS_TEST_DIRNAME}/update/www/10/Manifest.os-core)
  stampHash=$(awk '/versionstamp$/ {print $2}' ${BATS_TEST_DIRNAME}/update/www/10/Manifest.os-core)

  sudo sed -i "/os-release$/ s/[a-z0-9]\{64\}/$releaseHash/" ${BATS_TEST_DIRNAME}/update/www/20/Manifest.os-core
  sudo sed -i "/version$/ s/[a-z0-9]\{64\}/$versionHash/" ${BATS_TEST_DIRNAME}/update/www/20/Manifest.os-core
  sudo sed -i "/versionstamp$/ s/[a-z0-9]\{64\}/$stampHash/" ${BATS_TEST_DIRNAME}/update/www/20/Manifest.os-core

  run sudo mixer build validate --from 10 --to 20 --native

  [[ $status -ne 0 ]]

  # Check for inserted file failures
  [[ $output =~ "ERROR: /fakeAdd is added in manifest 'os-core', but not in a package" ]]
  [[ $output =~ "ERROR: /fakeDel is deleted in manifest 'os-core', but not in a package" ]]
  [[ $output =~ "ERROR: /fakeMod is modified in manifest 'os-core', but not in a package" ]]
  [[ $output =~ "ERROR: /usr/lib/os-release is not modified in manifest 'os-core'" ]]
  [[ $output =~ "ERROR: /usr/share/clear/version is not modified in manifest 'os-core'" ]]
  [[ $output =~ "ERROR: /usr/share/clear/versionstamp is not modified in manifest 'os-core'" ]]
}

@test "MCA compare to +10 error handling" {
  # Prevent mandatory format file modification to create special case error
  formatHash=$(awk '/swupd\/format$/ {print $2}' "${BATS_TEST_DIRNAME}"/update/www/10/Manifest.os-core-update)
  sudo sed -i "/swupd\/format$/ s/[a-z0-9]\{64\}/$formatHash/" ${BATS_TEST_DIRNAME}/update/www/30/Manifest.os-core-update
  
  run sudo mixer build validate --from 20 --to 30 --native

  [[ $status -ne 0 ]]
  [[ $output =~ "ERROR: /usr/share/defaults/swupd/format is not modified in manifest 'os-core-update'" ]]
}

@test "MCA compare +10 to +20 error handling" {
  # Insert special case file change to generate warning message
  hash1=d93a5e9123361e28b9e244fe422234e3a1794b001a082aeb78e16fd881673a2c
  sudo sed -i "/os-release$/ s/[a-z0-9]\{64\}/$hash1/" ${BATS_TEST_DIRNAME}/update/www/40/Manifest.os-core

  run sudo mixer build validate --from 30 --to 40 --native

  [[ $status -ne 0 ]]
  [[ $output =~ "WARNING: If this is a +10 to +20 comparison, os-core/os-core-update have file exception errors" ]]
}

@test "MCA override DNF conf to invalid URL" {
  # Override upstream repo to invalid URL to confirm DNF conf overrides succeed
  run sudo sh -c "mixer build validate --from 10 --to 20 --native --from-repo-url clear=foo"

  [[ "$status" -ne 0 ]]
  [[ "$output" =~ "RPM download attempt 4 failed. Maximum of 4 attempts." ]]
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
