# NOTE: source this file from a BATS test case file

# This library defines functions to use in the BATS test files during a local
# test run with 'make check'.

export cachedir="$HOME/.cache/mixer-tests"
logdir="$BATS_TEST_DIRNAME/logs"
BUNDLE_DIR="$BATS_TEST_DIRNAME/local-bundles"
CLRVER=$(curl https://download.clearlinux.org/latest)
CLR_BUNDLES="$BATS_TEST_DIRNAME/upstream-bundles/clr-bundles-$CLRVER/bundles"
BUNDLE_LIST="$BATS_TEST_DIRNAME/mixbundles"
mkdir -p $cachedir
mkdir -p $logdir

setup_builder_conf() {
:
}

localize_builder_conf() {
  echo "LOCAL_RPM_DIR = $BATS_TEST_DIRNAME/local-rpms
LOCAL_REPO_DIR = $BATS_TEST_DIRNAME/local-yum" | sudo tee -a $BATS_TEST_DIRNAME/builder.conf > /dev/null
}

mixer-init-versions() {
  sudo touch $BATS_TEST_DIRNAME/mixbundles
  sudo -E mixer init --clear-version $1 --mix-version $2 --new-swupd
  sudo sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf
}

clean-bundle-dir() {
  sudo rm -rf $BUNDLE_DIR/* $BATS_TEST_DIRNAME/mixbundles
  echo -e "filesystem\n" | sudo tee $BUNDLE_DIR/os-core > /dev/null
  sudo mixer bundle add os-core
}

mixer-build-bundles() {
  sudo -E mixer build bundles --config $BATS_TEST_DIRNAME/builder.conf --new-swupd --new-chroots
}

mixer-create-update() {
  sudo -E mixer build update --config $BATS_TEST_DIRNAME/builder.conf --new-swupd
}

mixer-add-rpms() {
  mkdir -p $BATS_TEST_DIRNAME/local-yum $BATS_TEST_DIRNAME/local-rpms
  sudo -E mixer add-rpms --config $BATS_TEST_DIRNAME/builder.conf --new-swupd
}

add-bundle() {
  sudo touch $BUNDLE_DIR/$1
}

add-package() {
  echo $1 | sudo tee -a $BUNDLE_DIR/$2 > /dev/null
  sudo mixer bundle add $2
}

add-clear-bundle() {
  sudo cp $CLR_BUNDLES/$1 $BUNDLE_DIR
  sudo mixer bundle add $1
}

remove-bundle() {
  sudo sed -i "/$1/d" $BUNDLE_LIST
}

remove-package() {
  sudo sed -i "/$1/d" $BUNDLE_DIR/$2
}

download-rpm() {
  mkdir -p $BATS_TEST_DIRNAME/local-rpms
  pushd $BATS_TEST_DIRNAME/local-rpms
  sudo curl -LO $1
  popd
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
