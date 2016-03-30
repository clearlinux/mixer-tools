#!/bin/bash
set -e

CLRVER=$(cat "$PWD/.clear-version")
MIXVER=$(cat "$PWD/.mix-version")
PREFIX=
LOG_DIR="$PWD/logs"

# Strip the trailing and leading whitespace on variables to sanitize them
function strip_whitespace {
    sed 's/ *$//' | sed 's/^ *//'
}

while [[ $# > 0 ]]
do
  key="$1"
  case $key in
    -c|--config)
    BUILDERCONF="$2"
    shift
    ;;
    -f|--format)
    FORMAT="$(echo $2 | strip_whitespace)"
    shift
    ;;
    -p|--prefix)
    PREFIX="$2"
    shift
    ;;
    -h|--help)
    echo -e "Usage: mixer-create-update.sh\n"
    echo -e "\t-c, --config Supply specific builder.conf\n"
    echo -e "\t-f, --format Supply format to use\n"
    echo -e "\t-p, --prefix Supply prefix for where the swupd binaries live\n"
    exit
    ;;
    *)
    echo -e "Invalid option\n"
    exit
    ;;
esac
shift
done

if [ ! -z "$BUILDERCONF" ]; then
    STATE_DIR=$(grep STATE_DIR "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
    BUNDLE_DIR=$(grep BUNDLE_DIR "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
    if [ -z "$FORMAT" ]; then
        FORMAT=$(grep FORMAT "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
    fi
elif [ -e "/etc/bundle-chroot-builder/builder.conf" ]; then
    STATE_DIR=$(grep STATE_DIR "/etc/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    BUNDLE_DIR=$(grep BUNDLE_DIR "/etc/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    if [ -z "$FORMAT" ]; then
            FORMAT=$(grep FORMAT "/etc/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    fi
else
    STATE_DIR=$(grep STATE_DIR "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    BUNDLE_DIR=$(grep BUNDLE_DIR "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    if [ -z "$FORMAT"]; then
            FORMAT=$(grep FORMAT "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    fi
fi

if [ -z "$FORMAT" ]; then
    FORMAT="staging"
fi

export BUNDLEREPO="$BUNDLE_DIR"

if [ ! -d "$STATE_DIR/www/version/format$FORMAT" ]; then
    sudo -E mkdir -p "$STATE_DIR/www/version/format$FORMAT/"
fi

# step 1: create update content for current mix
sudo -E "$PREFIX"swupd_create_update -S "$STATE_DIR" --osversion $MIXVER

# step 2: create fullfiles
sudo -E "$PREFIX"swupd_make_fullfiles -S "$STATE_DIR" $MIXVER

# step 3: create zero/delta packs
for bundle in $(ls "$BUNDLEREPO"); do
	sudo -E "$PREFIX"swupd_make_pack -S "$STATE_DIR" 0 $MIXVER $bundle
done

# step 4: hardlink relevant dirs
sudo -E "hardlink" -f "$STATE_DIR/image/$MIXVER"/*

# step 5: update latest version
sudo cp "$PWD/.mix-version" "$STATE_DIR/image/latest.version"
sudo cp "$PWD/.mix-version" "$STATE_DIR/www/version/format$FORMAT/latest"

# step 6: archive the swupd-server logs for this mix build
mkdir -p "$LOG_DIR/$MIXVER"
mv -f "$PWD"/swupd-*-$MIXVER.log "$LOG_DIR/$MIXVER/"
