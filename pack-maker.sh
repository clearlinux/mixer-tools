#!/bin/bash -e
# usage:
#	mixer-pack-maker.sh <to version> <back_count>
#	mixer-pack-maker.sh 2820 2
#	mixer-pack-maker.sh 2730 3
#
# For each bundle changed as of the Manifest.MoM at the <to version>,
# create version-pair packs going back <back_count>
# versions of that bundle.  For example if bundle "editors" changed in 2650
# and the script is run as "pack_maker.sh 2650 2", it will find that
# "editors" last two change points were 2550 and 2260 and thus create a
# 2550-to-2650 and a 2260-to-2650 version pair pack for the "editors"
# bundle.

#NOTE: fullfiles must be created before calling this script
#NOTE: zero packs need similarly created

#NOTE: build numbers are sparse, search back for the next oldest existing build
#NOTE: in the future isn't simply -10, and when that change happens we must also
#      consider which version-pair packs are meaningful for intermediate builds
#      and revert builds.

export XZ_DEFAULTS="--threads 0"

VER=$1
BACK_COUNT=$2
UPDATEDIR=$3
SWUPDREPO=${SWUPDREPO}

if [ -z "${BACK_COUNT}" ]; then
	echo "missing to pack count"
	exit 1
fi

if [ -z "${VER}" ]; then
	echo "missing to version"
	exit 1
fi

if [ -z "${UPDATEDIR}" ]; then
	echo "Missing STATE_DIR, using /var/lib/update/"
	UPDATEDIR="/var/lib/update/"
fi

# So we can call swupd_* with a sane path even if it's not / terminated
if [ ! -z "${SWUPDREPO}" ]; then
	SWUPDREPO="${SWUPDREPO}/"
fi

SWUPDWEBDIR="${UPDATEDIR}/www"

MOM=${SWUPDWEBDIR}/${VER}/Manifest.MoM
if [ ! -e ${MOM} ]; then
	echo "invalid version (no ${MOM})"
	exit 1
fi

BUNDLE_LIST=$(cat ${MOM} | awk -v V=${VER} '$1 ~ /^M\./ && $3 == V { print $4 }')

# build packs for all bundles changed in $VER
for BUNDLE in $BUNDLE_LIST; do
	BUNDLE_VER_LIST=""
	MANIFEST_VER=$(cat ${SWUPDWEBDIR}/$VER/Manifest.MoM | grep "^previous:" | cut -f 2)
	BUNDLE_VER=""
	COUNT=0
	while [ $COUNT -lt $BACK_COUNT ] && [ $MANIFEST_VER -gt 0 ]; do
		MOM=${SWUPDWEBDIR}/$MANIFEST_VER/Manifest.MoM
		if [ -e ${MOM} ]; then
			BUNDLE_VER=$(cat ${MOM} | grep "	${BUNDLE}$" | cut -f 3)
			if [ "$BUNDLE_VER" != "" ]; then
				BUNDLE_VER_LIST="$BUNDLE_VER $BUNDLE_VER_LIST"
				MANIFEST_VER=$(cat ${SWUPDWEBDIR}/$BUNDLE_VER/Manifest.MoM | grep "^previous:" | cut -f 2)
				let COUNT=$((COUNT+1))
			else
				# back in history to where bundle didn't exist yet
				MANIFEST_VER=0
			fi
		else
			# seek backwards until a manifest if found
			MANIFEST_VER=$((MANIFEST_VER-10))
		fi
	done

	for v in $BUNDLE_VER_LIST; do
		if [ -e ${SWUPDWEBDIR}/${VER}/pack-${BUNDLE}-from-$v.tar ]; then
			echo "${VER}/pack-${BUNDLE}-from-$v.tar already exists, skipping."
		else
			${SWUPDREPO}swupd_make_pack --statedir ${UPDATEDIR} $v ${VER} ${BUNDLE} &
		fi
	done
done

for job in $(jobs -p)
do
	wait ${job}
	RET=$?
	if [ "$RET" != "0" ]; then
		echo "pack creation failure!"
		exit $RET
	fi
done

# NOTE: your devops will want to expose the completed swupd server build to
# trial usage at some point.  This would be done via code like:
#
#	echo ${VER} > ${SWUPDWEBDIR}/../image/LAST_VER
#	STAGING=$(cat ${SWUPDWEBDIR}/version/formatstaging/latest)
#	if [ "${STAGING}" -lt "${VER}" ]; then
#		echo ${VER} > ${SWUPDWEBDIR}/version/formatstaging/latest
#	fi
