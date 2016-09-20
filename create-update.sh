#!/bin/bash
set -e

PREFIX=
LOG_DIR="$PWD/logs"
NOPUBLISH=0
ZEROPACKS=1
KEEP_CHROOTS=0

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
		-m|--minversion)
		MINVERSION="$2"
		shift
		;;
		-p|--prefix)
		PREFIX="$2"
		shift
		;;
		-z|--no-zero-packs)
		ZEROPACKS=0
		;;
		--no-publish)
		NOPUBLISH=1
		;;
		--keep-chroots)
		KEEP_CHROOTS=1
		;;
		-h|--help)
		echo -e "Usage: mixer-create-update.sh\n"
		echo -e "\t-c, --config Supply specific builder.conf\n"
		echo -e "\t-f, --format Supply format to use\n"
		echo -e "\t-m, --minversion supply minversion to build upate with\n"
		echo -e "\t-p, --prefix Supply prefix for where the swupd binaries live\n"
		echo -e "\t    --no-publish Do not update the latest version after update \n"
		exit
		;;
		*)
		echo -e "Invalid option\n"
		exit
		;;
	esac
	shift
done

CLRVER=$(cat "$PWD/.clear-version")
MIXVER=$(cat "$PWD/.mix-version")

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
sudo -E "$PREFIX"swupd_create_update -S "$STATE_DIR" --minversion "$MINVERSION" -F "$FORMAT" --osversion $MIXVER

# we only need the full chroot from this point on, so cleanup the others
if [  "$KEEP_CHROOTS" -eq 0 ]; then
		for BUNDLE in $(ls "$BUNDLEREPO" | grep -v "^full"); do
			sudo rm -rf "$STATE_DIR/image/$MIXVER/$BUNDLE"
		done
fi

# step 2: create fullfiles
sudo -E "$PREFIX"swupd_make_fullfiles -S "$STATE_DIR" $MIXVER

# step 3: create zero packs
MOM="$STATE_DIR/www/$MIXVER/Manifest.MoM"
if [ ! -e ${MOM} ]; then
    error "no ${MOM}"
fi
BUNDLE_LIST=$(cat ${MOM} | awk -v V=${MIXVER} '$1 ~ /^M\./ && $3 == V { print $4 }')
# NOTE: for signing, pass the --signcontent option to swupd_make_pack.
# Signing is currently disabled until there are new test certs ready.
if [ $ZEROPACKS -eq 1 ]; then
	for BUNDLE in $BUNDLE_LIST; do
		sudo -E "$PREFIX"swupd_make_pack -S "$STATE_DIR" 0 $MIXVER $BUNDLE &
	done

	for job in $(jobs -p); do
	    wait ${job}
	    RET=$?
	    if [ "$RET" != "0" ]; then
		error "zero pack subprocessor failed"
	    fi
	done
fi

# step 4: hardlink relevant dirs
sudo -E "hardlink" -f "$STATE_DIR/image/$MIXVER"/

# step 5: update latest version
if [ $NOPUBLISH -eq 0 ]; then
	sudo cp "$PWD/.mix-version" "$STATE_DIR/image/latest.version"
	sudo cp "$PWD/.mix-version" "$STATE_DIR/www/version/format$FORMAT/latest"
fi

# step 6: archive the swupd-server logs for this mix build
mkdir -p "$LOG_DIR/$MIXVER"
mv -f "$PWD"/swupd-*-$MIXVER.log "$LOG_DIR/$MIXVER/"
