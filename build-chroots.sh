#!/bin/bash

BUILDERSCRIPT="bundle-chroot-builder.py"
CLRVER=$(cat "$PWD/.clear-version")
MIXVER=$(cat "$PWD/.mix-version")

while [[ $# > 0 ]]
do
    key="$1"
    case $key in
        -c|--config)
        BUILDERCONF="$2"
        shift
        ;;
        -h|--help)
        echo -e "Usage: mixer-build-chroots.sh\n"
        echo -e "\t-c, --config Supply specific builder.conf\n"
        exit
        ;;
        *)
        echo -e "Invalid option\n"
        exit
        ;;
    esac
    shift
done

# FIXME: For now, only build chroots for the mix. In the future, when we run the
# ABI checker and build_comp to catch additional problems compared to upstream
# Clear, we will need to build vanilla Clear chroots as well.
BUILDTYPE="mix"

# FIXME: LANG is set correctly as root user, but not otherwise.
# Desperately needs a fix in Clear...
export LANG="en_US.utf8"

check_dep() {
    type $1 &> /dev/null
    if [ $? -ne 0 ]; then
        echo "$1 program not found... Unable to continue"
        exit 1
    fi
}

# Check dependencies

# check_dep "abi-compliance-checker"
# check_dep "bundle-chroot-builder"
check_dep "cp"
check_dep "hardlink"
check_dep "m4"
check_dep "rpm"
check_dep "yum"

if [ ! -e "$PWD/yum.conf.in" ]; then
    cp /usr/share/defaults/mixer/yum.conf.in .
fi

# Strip the trailing and leading whitespace on variables to sanitize them
function strip_whitespace {
    sed 's/ *$//' | sed 's/^ *//'
}

# Read values from builder.conf, either supplied or default
if [[ ! -z $BUILDERCONF ]]; then
    STATE_DIR=$(grep STATE_DIR "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
    YUM_CONF=$(grep YUM_CONF "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
    CERT=$(grep CERT "$BUILDERCONF" | cut -d "=" -f2 | strip_whitespace)
elif [ -e "/etc/bundle-chroot-builder/builder.conf" ]; then
    STATE_DIR=$(grep STATE_DIR "/etc/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    YUM_CONF=$(grep YUM_CONF "/etc/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    CERT=$(grep CERT "/etc/bundle-chroot-builder/builder.conf"| cut -d "=" -f2 | strip_whitespace)
else
    STATE_DIR=$(grep STATE_DIR "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    YUM_CONF=$(grep YUM_CONF "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
    CERT=$(grep CERT "/usr/share/defaults/bundle-chroot-builder/builder.conf" | cut -d "=" -f2 | strip_whitespace)
fi

if [ "$BUILDTYPE" = "clear" ]; then
    m4 yum.conf.in > "$PWD/.yum-clear.conf"
    BUILDVER=$CLRVER
elif [ "$BUILDTYPE" = "mix" ]; then
    if [ ! -f "$PWD/.mixer-repopath" ] ; then
        m4 yum.conf.in > "$PWD/.yum-mix.conf"
    else
        repopath=$(cat "$PWD/.mixer-repopath" | xargs realpath)
        m4 -D MIXER_REPO -D MIXER_REPOPATH="$repopath" yum.conf.in > "$PWD/.yum-mix.conf"
    fi
    BUILDVER=$MIXVER
fi

# if BUILDVER already exists wipe it so it's a fresh build
if [ -d $STATE_DIR/image/$BUILDVER ] ; then
    echo -e "Wiping away previous version $BUILDVER...\n"
    sudo -E rm -rf "$STATE_DIR/www/$BUILDVER"
    sudo -E rm -rf "$STATE_DIR/image/$BUILDVER"
fi

# if this is a mix, need to build with the Clear version, but publish the mix version
if [[ ! -z $BUILDERCONF ]]; then
    sudo -E sh -c "LD_PRELOAD=/usr/lib64/nosync/nosync.so $BUILDERSCRIPT -c $BUILDERCONF -m $BUILDVER $CLRVER"
else
    sudo -E sh -c "LD_PRELOAD=/usr/lib64/nosync/nosync.so $BUILDERSCRIPT -m $BUILDVER $CLRVER"
fi

# Create the certificate needed for signing verification if it does not exist, and then
# insert it into the chroot
if [ ! -z $CERT ]; then
    if [ ! -f $CERT ]; then
        # This generates the private key and self signed certificate
        openssl req -x509 -sha256 -nodes -days 365 -newkey rsa:2048 \
                -keyout private.pem -out $CERT \
                -subj "/C=US/ST=Oregon/L=Portland/O=Company Name/OU=Org/CN=www.example.com/DN=MixerCert" \
                -config /usr/share/defaults/mixer/certattributes.cnf
    fi
    echo -e "Installing certificate\n"
    sudo cp $CERT "$STATE_DIR/image/$BUILDVER/os-core-update/usr/share/clear/update-ca/"
fi

# clean up the files-* entries since they are now copied into the webdir noship
for i in $(ls $STATE_DIR/image/$MIXVER | grep files-*);
do
    sudo rm -f $STATE_DIR/image/$MIXVER/$i;
done;
# vi: ts=8 sw=4 sts=4 et tw=80
