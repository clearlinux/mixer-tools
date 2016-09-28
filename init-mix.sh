#!/bin/bash

set -e

if [ -e /usr/lib/os-release ]; then
    CLRVER=$(awk -F= '/^VERSION_ID/ { print $2 }' /usr/lib/os-release)
fi

ALL=0
MIXVER=

while [[ $# > 0 ]]
do
    key="$1"
    case $key in
        -c|--config)
        BUILDERCONF="$2"
        shift
        ;;
        -b|--clear-version)
        CLRVER="$2"
        shift
        ;;
        -m|--mix-version)
        MIXVER="$2"
        shift
        ;;
        -a|--all-bundles)
        ALL=1
        ;;
        -h|--help)
        echo -e "Usage: mixer-init-mix.sh\n"
        echo -e "\t-c, --config Supply specific builder.conf\n"
        echo -e "\t-b, --clear-version Supply specific Clear version to build against\n"
        echo -e "\t-m, --mix-version Supply the specific Mix version to build\n"
        echo -e "\t-a, --all-bundles Create a mix with all Clear bundles included\n"
        exit
        ;;
        *)
        echo -e "Invalid option\n"
        exit
        ;;
    esac
    shift
done

if [ -z "$CLRVER" ]; then
    echo -e "Please supply Clear version to use\n"
    exit
fi

if [ -z "$MIXVER" ]; then
    MIXVER=10
fi

echo -e "Creating initial update version $MIXVER\n"

mixer-init-versions.sh -m $MIXVER -c $CLRVER
mixer-update-bundles.sh

# Do not build the update content unless the --all-bundles flag is passed, user may want
# to do additional changes to the bundles for the first version.
if [ $ALL -eq 0 ]; then
    echo -e "Initializing mix with bundles:\n* os-core\n* os-core-update\n* bootloader\n* kernel-native\n"
    cd mix-bundles/
    rm -rf *
    git checkout os-core os-core-update bootloader kernel-native
    git add .
    git commit -s -m "Prune bundles for starting version $MIXVER"
    cd -
else
    if [[ ! -z $BUILDERCONF ]]; then
        mixer-build-chroots.sh -c $BUILDERCONF
        mixer-create-update.sh -c $BUILDERCONF
    else
        mixer-build-chroots.sh
        mixer-create-update.sh
    fi
fi
# vi: ts=8 sw=4 sts=4 et tw=80
