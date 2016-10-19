#!/bin/bash
if [ ! -f /usr/share/mixer-tools/helpers ]; then
    echo "Cannot find /usr/share/mixer-tools/helpers, please install first, exiting..."
    exit
fi
source /usr/share/mixer-tools/helpers
set -e

PREFIX=
LOG_DIR="$PWD/logs"
NOPUBLISH=0
ZEROPACKS=1
KEEP_CHROOTS=0

while [[ $# > 0 ]]
do
	key="$1"
	case $key in
		-c|--config)
		BUILDERCONF="$2"
		shift
		;;
		-f|--format)
		FORMAT="$2"
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
		echo -e "\t-c, --config\t\tSupply specific builder.conf\n"
		echo -e "\t-f, --format\t\tSupply format to use\n"
		echo -e "\t-m, --minversion\tSupply minversion to build upate with\n"
		echo -e "\t-p, --prefix Supply\tprefix for where the swupd binaries live\n"
		echo -e "\t    --no-publish\tDo not update the latest version after update\n"
		echo -e "\t    --keep-chroots\tKeep individual chroots created not just the consolidated 'full'"
		exit
		;;
		*)
		echo -e "Invalid option\n"
		exit
		;;
	esac
	shift
done

# Set the possible builder.conf files to read from
load_builder_conf
BUILDERCONFS="
$BUILDERCONF
$LOCALCONF
"

# Read values from builder.conf, either supplied or default
# This will prioritize reading from cmd line, etc, and then /usr/share/defaults/
read_builder_conf $BUILDERCONFS

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
	clean_chroots
fi

# step 1.5: sign the Manifest.MoM that was just created
sign_manifest_mom

# step 2: create fullfiles
sudo -E "$PREFIX"swupd_make_fullfiles -S "$STATE_DIR" $MIXVER

# step 3: create zero packs
if [ $ZEROPACKS -eq 1 ]; then
	sudo -E mixer-pack-maker.sh --to $MIXVER -S "$STATE_DIR"
fi

# step 4: hardlink relevant dirs
sudo -E "hardlink" -f "$STATE_DIR/image/$MIXVER"/

# step 5: update latest version
if [ $NOPUBLISH -eq 0 ]; then
	sudo -E echo "$MIXVER" > "$STATE_DIR/image/LAST_VER"
	sudo -E echo "$MIXVER" > "$STATE_DIR/www/version/format$FORMAT/latest"
fi

# step 6: archive the swupd-server logs for this mix build
mkdir -p "$LOG_DIR/$MIXVER"
mv -f "$PWD"/swupd-*-$MIXVER.log "$LOG_DIR/$MIXVER/"
