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
  mixer $MIXARGS config set Mixer.LOCAL_RPM_DIR $BATS_TEST_DIRNAME/local-rpms --native=true
  mixer $MIXARGS config set  Mixer.LOCAL_REPO_DIR $BATS_TEST_DIRNAME/local-yum --native=true
}

# Initializes a mix with the desired versions. Then for efficiency converts 
# builder.conf to use os-core for the "update bundle", strips os-core to just
# the filesystem, and adds only os-core to the mix
mixer-init-stripped-down() {
  mixer $MIXARGS init --clear-version $1 --mix-version $2 --no-default-bundles --native=true
  sed -i 's/os-core-update/os-core/' $BATS_TEST_DIRNAME/builder.conf
  echo "filesystem" > $LOCAL_BUNDLE_DIR/os-core
  mixer $MIXARGS bundle add os-core --native=true
}

mixer-versions-update() {
  mixer $MIXARGS versions update --mix-version $1 --upstream-version $2 --native=true
}
mixer-mixversion-update() {
  mixer $MIXARGS versions update --mix-version $1 --native=true
}

mixer-upstream-update() {
  mixer $MIXARGS versions update --upstream-version $1 --native=true
}

mixer-build-bundles() {
  sudo -E mixer $MIXARGS build bundles --config $BATS_TEST_DIRNAME/builder.conf --native=true
}

mixer-build-update() {
  sudo -E mixer $MIXARGS build update --config $BATS_TEST_DIRNAME/builder.conf --native=true
}

mixer-build-update-minversion() {
  sudo -E mixer $MIXARGS build update --config $BATS_TEST_DIRNAME/builder.conf --native=true --min-version $1
}

mixer-build-all() {
  sudo -E mixer $MIXARGS build all --config $BATS_TEST_DIRNAME/builder.conf --native=true
}

mixer-build-delta-packs() {
  sudo -E mixer $MIXARGS build delta-packs --config $BATS_TEST_DIRNAME/builder.conf --native=true --previous-versions $1
}

mixer-build-format-bump-new() {
  sudo -E mixer $MIXARGS build format-bump new --new-format $1 --native=true
}

mixer-build-format-bump-old() {
  sudo -E mixer $MIXARGS build format-bump old --new-format $1 --native=true
}

mixer-build-format-bump() {
  sudo -E mixer $MIXARGS build format-bump --new-format $1 --native=true
}

mixer-build-upstream-format-bump() {
  sudo -E mixer $MIXARGS build upstream-format --new-format $1 --native=true
}

mixer-add-rpms() {
  mkdir -p $BATS_TEST_DIRNAME/local-yum $BATS_TEST_DIRNAME/local-rpms
  sudo -E mixer $MIXARGS add-rpms --config $BATS_TEST_DIRNAME/builder.conf --native=true
}

create-empty-local-bundle() {
  mixer bundle edit $1
}

add-package-to-local-bundle() {
  echo $1 >> $LOCAL_BUNDLE_DIR/$2
}

remove-package-from-local-bundle() {
  sed -i "/$1/d" $LOCAL_BUNDLE_DIR/$2
}

mixer-bundle-add() {
  mixer $MIXARGS bundle add $1 --native=true
}

mixer-bundle-remove() {
  mixer $MIXARGS bundle remove $1 --native=true
}

get-current-version() {
  latest=$(curl https://download.clearlinux.org/latest)

  echo $latest
}

get-current-format() {
  latest=$(get-current-version)
  format=$(curl https://download.clearlinux.org/update/$latest/format)

  echo $format
}

get-last-format-boundary() {
  format=$(get-current-format)
  first=$(curl https://download.clearlinux.org/update/version/format$format/first)

  echo $(($first-10))
}

setup-dnf() {
  mkdir -p $BATS_TEST_DIRNAME/../../dnf

  [ -f $BATS_TEST_DIRNAME/../../dnf/dnf-mix.conf ] && return

  cat > $BATS_TEST_DIRNAME/../../dnf/dnf-mix.conf <<EOF
[main]
cachedir=$BATS_TEST_DIRNAME/../../dnf
keepcache=0
debuglevel=2
logfile=$BATS_TEST_DIRNAME/../../dnf
exactarch=1
obsoletes=1
gpgcheck=0
plugins=0

[clear]
name=Clear
failovermethod=priority
baseurl=https://download.clearlinux.org/releases/\$releasever/clear/x86_64/os/
enabled=1
gpgcheck=0
EOF
}

download-rpm() {
  version=$(get-current-version)

  setup-dnf

  sudo -E dnf install --config=$BATS_TEST_DIRNAME/../../dnf/dnf-mix.conf --downloadonly --releasever $version -y $1

  mkdir -p $BATS_TEST_DIRNAME/local-rpms
  cp $BATS_TEST_DIRNAME/../../dnf/clear*/packages/$1*.rpm $BATS_TEST_DIRNAME/local-rpms
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
