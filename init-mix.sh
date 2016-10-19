#!/bin/bash
if [ ! -f /usr/share/mixer-tools/helpers ]; then
    echo "Cannot find /usr/share/mixer-tools/helpers, please install first, exiting..."
    exit
fi
source /usr/share/mixer-tools/helpers
set -e

if [ -e /usr/lib/os-release ]; then
    CLRVER=$(awk -F= '/^VERSION_ID/ { print $2 }' /usr/lib/os-release)
fi

ALL=0

while [[ $# > 0 ]]
do
    key="$1"
    case $key in
        -c|--config)
        BUILDERCONF="$2"
        shift
        ;;
        -a|--all-bundles)
        ALL=1
        ;;
        -h|--help)
        echo -e "Usage: mixer-init-mix.sh\n"
        echo -e "\t-c, --config\t\tSupply specific builder.conf\n"
        echo -e "\t-a, --all-bundles\tCreate a mix with all Clear bundles included\n"
        exit
        ;;
        *)
        echo -e "Invalid option\n"
        exit
        ;;
    esac
    shift
done

# Check dependencies before doing any more work
check_deps

# Set the possible builder.conf files to read from
load_builder_conf
BUILDERCONFS="
$BUILDERCONF
$LOCALCONF
"

# Read values from builder.conf, either supplied or default
# This will prioritize reading from cmd line, etc, and then /usr/share/defaults/
read_builder_conf $BUILDERCONFS

echo -e "Creating initial update version $MIXVER\n"

if [[ ! -z $BUILDERCONF ]]; then
    mixer-update-bundles.sh -c $BUILDERCONF
elif [ -f $LOCALCONF ]; then
    mixer-update-bundles.sh -c $LOCALCONF
else
    mixer-update-bundles.sh
fi

# Do not build the update content unless the --all-bundles flag is passed, user may want
# to do additional changes to the bundles for the first version.
if [ $ALL -eq 0 ]; then
    echo -e "Initializing mix with bundles:\n* os-core\n* os-core-update\n* bootloader\n* kernel-native\n"
    cd mix-bundles/
    rm -rf *
    git checkout os-core os-core-update bootloader kernel-native
    git add .
    git commit -s -m "Prune bundles for initial version $MIXVER"
    cd -
else
    if [[ ! -z $BUILDERCONF ]]; then
        mixer-build-chroots.sh -c $BUILDERCONF
        mixer-create-update.sh -c $BUILDERCONF
    elif [ -f $LOCALCONF ]; then
        mixer-build-chroots.sh -c $LOCALCONF
        mixer-create-update.sh -c $LOCALCONF
    else
        mixer-build-chroots.sh
        mixer-create-update.sh
    fi
fi
# vi: ts=8 sw=4 sts=4 et tw=80
