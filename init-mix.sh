#!/bin/bash

set -e

if [ -e /usr/lib/os-release ]; then
        CLRVER=$(awk -F= '/^VERSION_ID/ { print $2 }' /usr/lib/os-release)
fi
if [[ ! -z $1 ]]; then
        CLRVER=$1
elif [[ -z $CLRVER ]]; then
        echo -e "Please supply Clear version to use\n"
        exit
fi

echo -e "Creating initial update version 10\n"

sudo -E sh -c "mixer-init-versions.sh -m 10 -c $CLRVER"

sudo -E "mixer-update-bundles.sh"

echo -e "Initializing mix with bundles:\n* os-core\n* os-core-update\n"
cd bundles/
sudo -E rm -rf *
sudo -E git checkout os-core os-core-update
sudo -E git add .
sudo -E git commit -s -m "Prune bundles for starting version 10"
cd -

sudo -E "mixer-build-chroots.sh"

sudo -E "mixer-create-update.sh"
# vi: ts=8 sw=2 sts=2 et tw=80
