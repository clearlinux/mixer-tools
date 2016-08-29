#!/bin/bash
#
# Usage:
#	mixer-pack-maker.sh --to VER [OPTIONS]
#
# Create zero packs and/or delta packs for a given swupd build VER.
#
# The presence of the --from option enables delta pack creation from the given
# version. More than one --from option may be specified. If both zero and delta
# packs should be created at the same time, pass the --zero option in addition
# to the --from option.
#
# To force recreation of all packs (zero and/or delta), use the --force option.
#
# Example invocations:
#
# 1) Create zero packs for build 7000
#
#     mixer-pack-maker.sh --to 7000
#
# 2) Create delta packs from build 6000 to 7000, and from 6500 to 7000
#
#     mixer-pack-maker.sh --to 7000 --from 6000 --from 6500
#
# 3) Create delta packs from build 6900 to 7000, and zero packs for build 7000
#
#     mixer-pack-maker.sh --to 7000 --from 6900 --zero
#

FROM_VERS=""
REPO_DIR=""
STATE_DIR=/var/lib/update
TO_VER=""
FORCE=0
ZERO_PACKS=0
PACK_LIST="$(mktemp pack-maker.XXXXXX)"

usage() {
	echo -e "Usage: mixer-pack-maker.sh --to VER [OPTIONS]\n"
	echo -e "\t-h, --help\tDisplay this help output\n"
	echo -e "\t-f, --from\tCreate delta packs using the given \"from\" version\n"
	echo -e "\t-R, --repodir\tLocation of swupd-server repo to find swupd_make_pack binary\n"
	echo -e "\t-S, --statedir\tThe state directory for swupd_make_pack. Defaults to /var/lib/update\n"
	echo -e "\t-t, --to\tCreate packs for the given version. This option is required.\n"
	echo -e "\t-x, --force\tRecreate packs if they already exist\n"
	echo -e "\t-z, --zero-packs\tCreate zero packs. If only --to is specified, this is the default.\n"
}

error() {
	echo -e "ERROR: $1\n"
}

check_mom() {
	local mom="$1"
	if [ ! -s "$mom" ]; then
		error "$mom not found"
		return 1
	fi
	return 0
}

create_pack() {
	local from="$1"
	local to="$2"
	local bundle="$3"

	if [ -s "$STATE_DIR"/www/${to}/pack-${bundle}-from-${from}.tar ] && [ "$FORCE" -eq 0 ]; then
		echo "${to}/pack-${bundle}-from-${from}.tar already exists, skipping."
	else
		"${REPO_DIR}"swupd_make_pack --statedir "${STATE_DIR}" ${from} ${to} ${bundle}
	fi
}

export -f create_pack

create_packs() {
	# The create_pack function uses these shell variables, so 'parallel'
	# needs them exported to the environment.
	export FORCE REPO_DIR STATE_DIR
	cat "$PACK_LIST" | parallel --colsep '\t' create_pack {1} {2} {3}
}

get_bundle_version() {
	local mom="$1"
	local bundle="$2"

	awk -v B=$bundle '$1 ~ /^M\./ && $4 == B { print $3 }' "$mom"
}

init_delta_packs() {
	local from_ver="$1"
	local from_mom="$STATE_DIR/www/$from_ver/Manifest.MoM"
	local to_mom="$STATE_DIR/www/$TO_VER/Manifest.MoM"

	check_mom $from_mom || return 1
	check_mom $to_mom || return 1

	awk '$1 ~ /^M\./ { print $3, $4 }' "$from_mom" | while read FROM BUNDLE; do
		local TO=$(get_bundle_version "$to_mom" "$BUNDLE")

		# If bundle does not exist in later version, skip
		[ -z "$TO" ] && continue

		# If versions are equal, skip
		[ "$FROM" -eq "$TO" ] && continue

		echo -e "${FROM}\t${TO}\t${BUNDLE}" >> "$PACK_LIST"
	done
}

init_zero_packs() {
	local to_mom="$STATE_DIR/www/$TO_VER/Manifest.MoM"

	check_mom $to_mom || return 1

	awk '$1 ~ /^M\./ { print $3, $4 }' "$to_mom" | while read TO BUNDLE; do
		echo -e "0\t${TO}\t${BUNDLE}" >> "$PACK_LIST"
	done
}

deduplicate_pack_list() {
	sort -u "$PACK_LIST" > "$PACK_LIST".new
	mv "$PACK_LIST".new "$PACK_LIST"
}

while [[ $# > 0 ]]; do
	key="$1"
	case $key in
		-f|--from)
		FROM_VERS="$FROM_VERS $2"
		shift
		;;
		-R|--repodir)
		REPO_DIR="$2"
		shift
		;;
		-S|--statedir)
		STATE_DIR="$2"
		shift
		;;
		-t|--to)
		TO_VER="$2"
		shift
		;;
		-x|--force)
		FORCE=1
		;;
		-z|--zero-packs)
		ZERO_PACKS=1
		;;
		-h|--help)
		usage
		exit 0
		;;
		*)
		error "Invalid option"
		echo ""
		exit 1
		;;
	esac
	shift
done

if [ -z "$TO_VER" ]; then
	error "Missing required --to option"
	echo ""
	usage
	exit 1
fi

if [ -z "$FROM_VERS" ]; then
	ZERO_PACKS=1
fi

# So we can call swupd_* with a sane path even if it's not / terminated
if [ -n "$REPO_DIR" ]; then
	d=$(dirname $REPO_DIR)
	b=$(basename $REPO_DIR)
	REPO_DIR="${d}/${b}/"
fi

# If delta packs are not being created, this loop does not execute
for ver in $FROM_VERS; do
	# The MoMs must exist; else skip this version pair
	init_delta_packs $ver || continue
done

if [ "$ZERO_PACKS" -eq 1 ]; then
	init_zero_packs
fi

deduplicate_pack_list

create_packs

rm "$PACK_LIST"


# NOTE: your devops will want to expose the completed swupd server build to
# trial usage at some point.  This would be done via code like:
#
#	echo ${TO_VER} > ${STATE_DIR}/image/LAST_VER
#	STAGING=$(cat ${STATE_DIR}/www/version/formatstaging/latest)
#	if [ "${STAGING}" -lt "${TO_VER}" ]; then
#		echo ${TO_VER} > ${STATE_DIR}/www/version/formatstaging/latest
#	fi
