#!/bin/bash

set -e

CLRVER=$(cat "$PWD/.clear-version")
GITREPODIR="$PWD/mix-bundles"

update_repo() {
    local repo="$1"
    (
    if [ ! -d "$repo" ]; then
        git clone https://github.com/clearlinux/"$repo.git"
        cd "$repo"
    else
        cd "$repo"
        # to force the update of clr-bundles "latest" tag
        git fetch --tags
        git checkout master
        git pull
    fi
    # checkout the tag relating to the clear version used to build against
    git checkout tags/"$CLRVER"
    set +e
    local branch="${CLRVER}_mix"
    git rev-parse --verify "$branch"
    if [ $? -eq 0 ]; then
        git checkout "$branch"
        git pull
    else
        git checkout -b "$branch"
    fi
    set -e
    cd ..
    ) &> /dev/null
    echo "$repo updated"
}

# Get the upstream clr-bundles
update_repo clr-bundles

# Set up mix bundle repo if it does not exist
if [ ! -d "$GITREPODIR" ]; then
    echo "Creating initial $GITREPODIR"
    mkdir "$GITREPODIR"
    cd "$GITREPODIR"
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
