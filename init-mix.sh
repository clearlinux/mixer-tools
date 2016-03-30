#!/bin/bash

set -e

if [ -e /usr/lib/os-release ]; then
        CLRVER=$(awk -F= '/^VERSION_ID/ { print $2 }' /usr/lib/os-release)
fi

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

sudo -E sh -c "mixer-init-versions.sh -m 10 -c $CLRVER"

sudo -E "mixer-update-bundles.sh"

echo -e "Initializing mix with bundles:\n* os-core\n* os-core-update\n"
cd bundles/
sudo -E rm -rf *
sudo -E git checkout os-core os-core-update
sudo -E git add .
sudo -E git commit -s -m "Prune bundles for starting version 10"
cd -

if [[ ! -z $BUILDERCONF ]]; then
	sudo -E sh -c "mixer-build-chroots.sh -c $BUILDERCONF"
else
	sudo -E "mixer-create-update.sh"
# vi: ts=8 sw=2 sts=2 et tw=80
