#!/bin/bash

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
        -b|--buildver)
        CLRVER="$2"
        shift
        ;;
        -a|--all-bundles)
        ALL=1
        ;;
        -h|--help)
        echo -e "Usage: mixer-init-mix.sh\n"
        echo -e "\t-c, --config Supply specific builder.conf\n"
        echo -e "\t-b, --buildver Supply specific Clear version to build against\n"
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

echo -e "Creating initial update version 10\n"

mixer-init-versions.sh -m 10 -c $CLRVER

mixer-update-bundles.sh
if [ $ALL -eq 0 ]; then
    echo -e "Initializing mix with bundles:\n* os-core\n* os-core-update\n"
    cd mix-bundles/
    rm -rf *
    git checkout os-core os-core-update
    git add .
    git commit -s -m "Prune bundles for starting version 10"
    cd -
fi

if [[ ! -z $BUILDERCONF ]]; then
    mixer-build-chroots.sh -c $BUILDERCONF
    mixer-create-update.sh -c $BUILDERCONF
else
    mixer-build-chroots.sh
    mixer-create-update.sh
fi
# vi: ts=8 sw=4 sts=4 et tw=80
