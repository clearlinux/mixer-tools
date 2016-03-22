#!/bin/bash
set -e

CLRVER=$(cat "$PWD/.clear-version")
MIXVER=$(cat "$PWD/.mix-version")
STATE_DIR=$(grep STATE_DIR "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | sed 's/ *//')
if [ -z "$1" ]; then
        FORMAT="staging"
else
        FORMAT=$1
fi
echo "format$FORMAT"

export BUNDLEREPO="$PWD/.repos/clr-bundles"

if [ ! -d "$STATE_DIR/www/version" ]; then
	mkdir -p "$STATE_DIR/www/version/formatstaging/"
	echo
fi

# step 1: create update content for current mix
sudo -E "swupd_create_update" -S "$STATE_DIR" --osversion $MIXVER

# step 2: create fullfiles
sudo -E "swupd_make_fullfiles" -S "$STATE_DIR" $MIXVER

# step 3: create zero/delta packs
for bundle in $(ls "$BUNDLEREPO/bundles/"); do
	sudo -E "swupd_make_pack" -S "$STATE_DIR" 0 $MIXVER $bundle
done

# step 4: hardlink relevant dirs
sudo -E "hardlink" -f "$STATE_DIR/image/$MIXVER"/*

# step 5: update latest version
sudo cp "$PWD/.mix-version" "$STATE_DIR/image/latest.version"
sudo cp "$PWD/.mix-version" "$STATE_DIR/www/version/format$FORMAT/latest"
# vi: ts=8 sw=2 sts=2 et tw=80
