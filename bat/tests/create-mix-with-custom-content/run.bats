#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Create stripped down mix 10 with custom content in custom bundle" {
  mixer-init-stripped-down $CLRVER 10
  localize_builder_conf

  download-rpm json-c
  mixer-add-rpms
  create-empty-local-bundle "testbundle"
  add-package-to-local-bundle "json-c" "testbundle"
  mixer-bundle-add "testbundle"

  mixer-build-bundles > $LOGDIR/build_bundles.log
  mixer-build-update > $LOGDIR/build_update.log
}

@test "Create stripped down mix 10 with custom rpm from custom repo" {
  mixer-init-stripped-down $CLRVER 10
  localize_builder_conf

  # create a custom repo directory which has a sub directory
  mkdir -p $BATS_TEST_DIRNAME/custom-yum/sub-dir

  # download the helloworld rpm, rename it to a non-autospec convention and place it within the repo sub directory
  curl --retry 4 --retry-delay 10 --fail --silent --show-error --location --remote-name https://download.clearlinux.org/releases/35110/clear/x86_64/os/Packages/helloworld-4-167.x86_64.rpm
  mv helloworld-4-167.x86_64.rpm $BATS_TEST_DIRNAME/custom-yum/sub-dir/foo.rpm

  # create the repo
  createrepo_c $BATS_TEST_DIRNAME/custom-yum/

  # add the custom repo to mixer with the baseurl not including the sub directory
  mixer repo add custom file://$BATS_TEST_DIRNAME/custom-yum

  # create and add a bundle for the downloaded package to the mix
cat > local-bundles/foobar <<EOF
# [TITLE]: foobar
# [DESCRIPTION]: example
# [STATUS]: Active
# [CAPABILITIES]: example
# [MAINTAINER]: example@example.com
helloworld
EOF
  mixer bundle add foobar

  # build the bundles
  mixer-build-bundles > $LOGDIR/build_bundles.log

  # check if the corresponding bin file for the package is created
  test -e $BATS_TEST_DIRNAME/update/image/10/full/usr/bin/helloworld
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
