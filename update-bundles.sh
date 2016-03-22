#!/bin/bash

set -e

CLRVER=$(cat "$PWD/.clear-version")
GITREPODIR="$PWD/.repos"

update_repo() {
  local repo="$1"
  echo "Updating $repo repo..."
  (
  if [ ! -d "$repo" ]; then
    git clone https://github.com/clearlinux/"$repo"
  else
    cd "$repo"
    # to force the update of clr-bundles "latest" tag
    git fetch --tags
    git checkout master
    git pull
    cd -
  fi
  if [ "$repo" = "clr-bundles" ]; then
    cd "$repo"
    # make sure the correct tag is checked out
    git checkout $CLRVER

    # Store mixer bundle modifications in a branch so that users can make
    # incremental changes over time. If rebasing on a new version of Clear, a
    # new branch is created, but the old branches remain, making it easier to
    # port bundle definitions.
    local branch="${CLRVER}_mix"
    set +e
    git rev-parse --verify "$branch"
    if [ $? -eq 0 ]; then
      git checkout "$branch"
    else
      git checkout -b "$branch"
    fi
    set -e
    cd -
  fi
  ) &> /dev/null
}

if [ ! -d "$GITREPODIR" ]; then
  mkdir "$GITREPODIR"
fi

cd "$GITREPODIR"
update_repo clr-bundles.git
cd - > /dev/null

# For easy visibility of the available bundles, create a directory symlink to
# the hidden repo location.
ln -sf "$GITREPODIR"/clr-bundles/bundles bundles

exit 0

# vi: ts=8 sw=2 sts=2 et tw=80
