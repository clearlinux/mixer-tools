#!/bin/bash
if [ ! -f /usr/share/mixer-tools/helpers ]; then
    echo "Cannot find /usr/share/mixer-tools/helpers, please install first, exiting..."
    exit
fi
source /usr/share/mixer-tools/helpers

set -e

function usage() {
    echo -e "Usage: $0\n"
    echo -e "\t-h, --help\t\tShow this menu\n"
    echo -e "\t-c, --config\t\tSupply specific builder.conf\n"
}


while [[ $# > 0 ]]
do
    key="$1"
    case $key in
        -c|--config)
        BUILDERCONF="$2"
        shift
        ;;
        -h | --help )
        usage; exit 1
        ;;
        *)
        usage; exit 1
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

if [ ! -d "$RPMDIR" ]; then
    echo "Path to directory containing RPMs must be specified in builder.conf"
    exit 1
fi

if [ ! -d "$REPODIR" ]; then
    echo "Path to local RPM repository must be specified in builder.conf"
    exit 1
fi

RPMS=$(find "$RPMDIR" -type f -name *.rpm)
if [ -z "$RPMS" ]; then
    echo "No RPMS found. Exiting"
    exit 1
fi

set +e
echo "$RPMS" | while read rpm; do
    ret=$(file "$rpm")
    echo "$ret" | grep --quiet ": RPM"
    if [[ $? -ne 0 ]] ; then
        echo "ERROR $rpm IS NOT VALID"
    else
        echo "Copying $rpm"
        cp "$rpm" "$REPODIR"
    fi
    done
set -e

( cd "$REPODIR" ; if type createrepo_c 1>/dev/null 2>&1; then createrepo_c .; else createrepo .; fi );

exit 0
