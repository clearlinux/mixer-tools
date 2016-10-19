#!/bin/bash
if [ ! -f /usr/share/mixer-tools/helpers ]; then
    echo "Cannot find /usr/share/mixer-tools/helpers, please install first, exiting..."
    exit
fi
source /usr/share/mixer-tools/helpers

set -e

while [[ $# > 0 ]]
do
    key="$1"
    case $key in
        -c|--config)
        BUILDERCONF="$2"
        shift
        ;;
        -h|--help)
        echo -e "Usage: mixer-update-bundles.sh\n"
        echo -e "\t-c, --config\t\tSupply specific builder.conf\n"
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

# Get the upstream clr-bundles
update_repo "clr-bundles"

# Set up mix bundle repo if it does not exist
if [ ! -d "$BUNDLE_DIR" ]; then
    echo "Creating initial $BUNDLE_DIR"
    mkdir "$BUNDLE_DIR"
    cd "$BUNDLE_DIR"
    (
    git init .
    cp ../clr-bundles/bundles/* .
    git add .
    git commit -s -m "Setup initial mixer bundles repo"
    cd -
    ) &> /dev/null
fi

exit 0
# vi: ts=8 sw=4 sts=4 et tw=80
