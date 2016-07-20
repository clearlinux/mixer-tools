#!/bin/bash
# Create super packs
#
# usage:
#
#	mixer-superpack-maker.sh <to version> <delta pack count> [<update directory>]
#
#	mixer-superpack-maker.sh 2820 5 /home/clr/mix/update
#		Creates superpack for version 2820. The superpack will replace the
#		oldest pack in that directory, excluding zero packs. 5 delta packs will
#		remain unchanged, the remaining delta packs will by symlinked to the
#		superpack. This script will use /home/clr/mix/update for the update
#		directory.
#
#	mixer-superpack-maker.sh 2820 5
#		Same as above except the update dir will default to /var/lib/update

# NOTE: delta packs must be created by calling mixer-pack-maker.sh before
#       calling this script.

if [ -z "$1" ]; then
	echo "$0 - missing to version"
	exit 1
fi

if [ -z "$2" ]; then
	echo "$0 - missing delta pack count"
	exit 1
fi

TO_VER=$1                           # $MIXVER in create_update.sh
DELTA_PACK_COUNT=$2                 # $DELTA_PACK_COUNT in create_update.sh
UPDATE_DIR=${3:-"/var/lib/update"}  # Defaults to /var/lib/update

WORK_DIR=$UPDATE_DIR/www/$TO_VER
CUR_DIR=$PWD

# Measure size of all packs before superpack created, report to stdout
INIT_SIZE=`sudo du -sck $WORK_DIR/pack* | tail -1 | awk {'print $1'}`
echo "Initial size before super pack created: $INIT_SIZE kB"

# Get a list of all bundles
MOM=$WORK_DIR/Manifest.MoM
BUNDLE_LIST=$(cat ${MOM} | awk -v V=${TO_VER} '$1 ~ /^M\./ && $3 == V { print $4 }')

for BUNDLE in $BUNDLE_LIST; do
	echo "Creating superpack for $BUNDLE"
	VER_LIST=()

	# Get a list of all packs except 0 pack
	VER_LIST=`ls $WORK_DIR | egrep -o "pack-$BUNDLE-from-[0-9]+.tar" | egrep -o '[1-9][0-9]*0'`
	# Store in temporary file so we can version sort
	for v in ${VER_LIST[@]}; do echo $v >> $WORK_DIR/tempver; done

	if [ -e ${WORK_DIR}/tempver ]; then
		VER_LIST=`sort --version-sort $WORK_DIR/tempver`
		sudo rm $WORK_DIR/tempver
	fi
	VER_LIST=($VER_LIST)

	# Make the super pack
	# untar into superpack/delta and superpack/staged directory
	# all delta files have different names to they won't be overwritten
	# duplicate fullfiles are overwritten - this is where we save space
	sudo mkdir -p "$WORK_DIR/superpack"/{"delta","staged"}
	sudo mkdir $WORK_DIR/tmp
	for v in ${VER_LIST[@]}; do
		# untar into a tmp directory so we can move everything over
		sudo tar -xf "$WORK_DIR/pack-$BUNDLE-from-$v.tar" -C $WORK_DIR/tmp
		mv "$WORK_DIR/tmp/delta"/* "$WORK_DIR/superpack/delta" 2>/dev/null
		mv "$WORK_DIR/tmp/staged"/* "$WORK_DIR/superpack/staged" 2>/dev/null
	done
	sudo rm -rf $WORK_DIR/tmp

	# tar everything in the superpack into a tar file named for the oldest
	# existing delta pack in the directory.
	cd $WORK_DIR/superpack
	sudo tar -cf "pack-$BUNDLE-from-${VER_LIST[0]}.tar" *
	sudo mv "pack-$BUNDLE-from-${VER_LIST[0]}.tar" $WORK_DIR/
	cd $CUR_DIR
	sudo rm -rf $WORK_DIR/superpack

	# remove unnecessary delta packs and create symlinks to the superpack with
	# the same name as the removed delta pack.
	let "NUM_PACKS = ${#VER_LIST[@]}"
	let "END_BRIDGE = $NUM_PACKS - $DELTA_PACK_COUNT"
	SUPERPACK="$WORK_DIR/pack-$BUNDLE-from-${VER_LIST[0]}.tar"
	i=1
	for v in ${VER_LIST[@]:1}; do
		if [ $i -lt $END_BRIDGE ] && [ $NUM_PACKS -gt 1 ]; then
			sudo rm "$WORK_DIR/pack-$BUNDLE-from-$v.tar"
			sudo ln -s "$SUPERPACK" "$WORK_DIR/pack-$BUNDLE-from-$v.tar"
		fi
		let "i++"
	done

	# finally, compress the superpack and remove the .xz extension
	sudo xz $SUPERPACK
	sudo mv "$SUPERPACK.xz" $SUPERPACK
done

# report end size and delta size
END_SIZE=`sudo du -sck $WORK_DIR/pack* | tail -1 | awk {'print $1'}`
echo "Pack size after super pack created: $END_SIZE kB"

let "DELTA_SIZE = $END_SIZE - $INIT_SIZE"
echo "Total delta: $DELTA_SIZE kB"
