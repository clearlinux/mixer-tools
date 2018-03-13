# NOTE: source this file from a BATS test case file

# This library defines functions to use in the BATS test files during a local
# test run with 'make check'.

LOGDIR="$BATS_TEST_DIRNAME/logs"
LOCAL_BUNDLE_DIR="$BATS_TEST_DIRNAME/local-bundles"
CLRVER=$(curl https://download.clearlinux.org/latest)
CLR_BUNDLES="$BATS_TEST_DIRNAME/upstream-bundles/clr-bundles-$CLRVER/bundles"
BUNDLE_LIST="$BATS_TEST_DIRNAME/mixbundles"
mkdir -p $LOGDIR

global_setup() {
: # Put content here for it to run for all tests
}

localize_builder_conf() {
  echo -e "LOCAL_RPM_DIR=$BATS_TEST_DIRNAME/local-rpms\nLOCAL_REPO_DIR=$BATS_TEST_DIRNAME/local-yum" >> $BATS_TEST_DIRNAME/builder.conf
}

# Initializes a mix with the desired versions. Then for efficiency converts 
# builder.conf to use os-core for the "update bundle", strips os-core to just
# the filesystem, and adds only os-core to the mix
mixer-init-stripped-down() {
  touch $BATS_TEST_DIRNAME/mixbundles
  mixer init --clear-version $1 --mix-version $2 --new-swupd
  sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf
  echo "filesystem" > $LOCAL_BUNDLE_DIR/os-core
  mixer bundle add os-core
}

mixer-versions-update() {
  mixer versions update --mix-version $1
}

mixer-build-bundles() {
  sudo -E mixer build bundles --config $BATS_TEST_DIRNAME/builder.conf --new-swupd --new-chroots
}

mixer-build-update() {
  sudo -E mixer build update --config $BATS_TEST_DIRNAME/builder.conf --new-swupd
}

mixer-add-rpms() {
  mkdir -p $BATS_TEST_DIRNAME/local-yum $BATS_TEST_DIRNAME/local-rpms
  sudo -E mixer add-rpms --config $BATS_TEST_DIRNAME/builder.conf --new-swupd
}

create-empty-local-bundle() {
  touch $LOCAL_BUNDLE_DIR/$1
}

add-package-to-local-bundle() {
  echo $1 >> $LOCAL_BUNDLE_DIR/$2
}

remove-package-from-local-bundle() {
  sed -i "/$1/d" $LOCAL_BUNDLE_DIR/$2
}

mixer-bundle-add() {
  mixer bundle add $1
}

mixer-bundle-remove() {
  mixer bundle remove $1
}

download-rpm() {
  mkdir -p $BATS_TEST_DIRNAME/local-rpms
  pushd $BATS_TEST_DIRNAME/local-rpms
  curl -LO $1
  popd
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
