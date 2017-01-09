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
        -n|--no-signing)
        SIGNING=0
        ;;
        -i|--incrememnt)
        INC=1
        ;;
        -h|--help)
        echo -e "Usage: mixer-build-chroots.sh\n"
        echo -e "\t-c, --config\t\tSupply specific builder.conf\n"
        echo -e "\t-n, --no-signing\tDo not generate a certificate and do not sign the Manifest.MoM\n"
        echo -e "\t-i, --incrememnt\tAutomatically bump the version up by +10"
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

# Generate the yum config file if it does not exist
# This takes the template and adds the relevant local rpm repo path if needed
if [ ! -f $YUM_CONF ]; then
    if [ -z "$REPODIR" ] ; then
        m4 "$YUM_TEMPLATE" > "$YUM_CONF"
    else
        m4 -D MIXER_REPO -D MIXER_REPOPATH="$REPODIR" "$YUM_TEMPLATE" > "$YUM_CONF"
    fi
fi

if [ $INC -eq 1 ]; then
    let MIXVER+=10
    if [[ ! -z $BUILDERCONF ]]; then
        sed -i "s/^MIX_VERSION.*/MIX_VERSION=$MIXVER/" $BUILDERCONF
    elif [ -f $LOCALCONF ]; then
        sed -i "s/^MIX_VERSION.*/MIX_VERSION=$MIXVER/" $LOCALCONF
    fi
    echo -e "Inrementing Mix version to $MIXVER\n"
fi

# If MIXVER already exists wipe it so it's a fresh build
if [ -d $STATE_DIR/image/$MIXVER ]; then
    echo -e "Wiping away previous version $MIXVER...\n"
    sudo -E rm -rf "$STATE_DIR/www/$MIXVER"
    sudo -E rm -rf "$STATE_DIR/image/$MIXVER"
fi

# If this is a mix, we need to build with the Clear version, but publish the mix version
if [[ ! -z $BUILDERCONF ]]; then
    sudo -E sh -c "LD_PRELOAD=/usr/lib64/nosync/nosync.so $BUILDERSCRIPT -c $BUILDERCONF -m $MIXVER $CLRVER"
elif [ -f $LOCALCONF ]; then
    sudo -E sh -c "LD_PRELOAD=/usr/lib64/nosync/nosync.so $BUILDERSCRIPT -c $LOCALCONF -m $MIXVER $CLRVER"
else
    sudo -E sh -c "LD_PRELOAD=/usr/lib64/nosync/nosync.so $BUILDERSCRIPT -m $MIXVER $CLRVER"
fi

# Create the certificate needed for signing verification if it does not exist, and then
# insert it into the chroot
if [ $SIGNING -eq 1 ]; then
    install_cert
fi

# clean up the files-* entries since they are now copied into the noship dir
for i in $(ls $STATE_DIR/image/$MIXVER | grep files-*);
do
    sudo rm -f $STATE_DIR/image/$MIXVER/$i;
done;
# vi: ts=8 sw=4 sts=4 et tw=80
