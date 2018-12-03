#!/usr/bin/env bats


# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Test --offline flag for all commands" {
  format=$(get-current-format)
  version=$(get-current-version)

  download-rpm filesystem

  export https_proxy=0.0.0.0
  export http_proxy=0.0.0.0

  #root
  mixer --offline

  #help
  mixer --help --offline
  mixer help  --offline

  #version
  mixer --version --offline

  # init
  mixer init --clear-version $version --format $format --mix-version 10 --no-default-bundles --offline

  # Prep for build
  sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf
  echo "filesystem" > $LOCAL_BUNDLE_DIR/os-core
  mixer $MIXARGS bundle add os-core --offline

  # add-rpms
  mixer add-rpms --offline

  # bundle
  mixer bundle edit testbundle --offline
  echo filesystem > $LOCAL_BUNDLE_DIR/testbundle
  mixer bundle add testbundle --offline
  mixer bundle list --offline
  mixer bundle validate --offline --all-local
  mixer bundle remove testbundle --offline

  # config
  mixer config set Server.DEBUG_INFO_BANNED true --offline
  mixer config get Server.DEBUG_INFO_BANNED --offline
  mixer config convert --offline
  mixer config validate --offline

  #repo
  mixer repo add test testurl --offline
  mixer repo init --offline
  mixer repo exclude test test --offline
  mixer repo list --offline
  mixer repo set-url test testurl2 --offline
  mixer repo remove test --offline

  #build
  sudo mixer build all --native --offline
  mixer versions update --offline
  sudo mixer build bundles --native --offline
  sudo mixer build update --native --offline
  sudo mixer build delta-packs --native --previous-versions=1 --offline

  #versions
  mixer versions update --upstream-version=$version --mix-version=30 --offline

  #cleanup
  sudo rm -rf update/ Swupd_Root.pem private.pem
}

@test "Test mix initialized in offline mode" {
  format=$(get-current-format)
  version=$(get-current-version)

  export https_proxy=0.0.0.0
  export http_proxy=0.0.0.0

  # init
  mixer init --clear-version $version --format $format --mix-version 10 --no-default-bundles --offline

  # Prep for build
  sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf
  echo "filesystem" > $LOCAL_BUNDLE_DIR/os-core
  mixer $MIXARGS bundle add os-core

  # add-rpms
  mixer add-rpms

  # bundle
  mixer bundle edit testbundle
  echo filesystem > $LOCAL_BUNDLE_DIR/testbundle
  mixer bundle add testbundle
  mixer bundle list
  mixer bundle validate --all-local
  mixer bundle remove testbundle

  # config
  mixer config set Server.DEBUG_INFO_BANNED true
  mixer config get Server.DEBUG_INFO_BANNED
  mixer config convert
  mixer config validate

  #repo
  mixer repo add test testurl
  mixer repo init
  mixer repo exclude test test
  mixer repo list
  mixer repo set-url test testurl2
  mixer repo remove test

  #build
  sudo mixer build all --native
  sudo mixer versions update
  sudo mixer build bundles --native
  sudo mixer build update --native
  sudo mixer build delta-packs --native --previous-versions=1

  #versions
  mixer versions update --upstream-version=$version --mix-version=30
}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
