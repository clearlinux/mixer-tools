#!/bin/bash

###############################################################################
# test setup
###############################################################################

# test workspace
mkdir afb-test
pushd afb-test

# init minimal mix with original format
mixer init --no-default-bundles --format 1
# create bundle to be deleted
mixer bundle edit foo --suppress-editor
# mark as deleted
sed -i "s/\(# \[STATUS\]:\).*/\1 Deprecated/" local-bundles/foo
# add minimal bundles including deleted bundle
mixer bundle add os-core os-core-update foo

###############################################################################
# +0
#
# This is the last normal build in the original format. When doing a format
# bump this build already exists. Building it now because the test needs a
# normal starting version.
###############################################################################

# build bundles and updates regularly
mixer build bundles --native
mixer build update --native

###############################################################################
# +10
#
# This is the last build in the original format. At this point add ONLY the
# content relevant to the format bump to the mash to be used. Relevant content
# should be the only change.
#
# Using original mixer for this build (may have to check out pre-format-bump
# version)
###############################################################################

# update mixer to build version 20, which in our case is the +10
mixer versions update --mix-version 20
# build bundles normally. At this point the bundles to be deleted should still
# be part of the mixbundles list and the groups.ini
mixer build bundles --native
# remove all deleted bundles' content by replacing bundle-info files with empty
# directories. This causes mixer to fall back to reading content for those
# bundles from a chroot. The chroots for these bundles will be empty.
for i in $(grep -lir "\[STATUS\]: Deprecated" upstream-bundles/ local-bundles/); do
	b=$(basename $i)
	rm -f update/image/20/$b-info; mkdir update/image/20/$b
done
# Replace the +10 version in /usr/lib/os-release with +20 version and write the
# new format to the format file on disk.  This is so clients will already be on
# the new format when they update to the +10 because the content is the same as
# the +20.
sed -i 's/\(VERSION_ID=\).*/\130/' update/image/20/full/usr/lib/os-release
echo 2 > update/image/20/full/usr/share/defaults/swupd/format
# build update based on the modified bundle information. This is *not* a
# minversion and these manifests must be built with the mixer from the original
# format (if manifest format changes).
mixer build update --native


###############################################################################
# +20
#
# This is the first build in the new format. The content is the same as the +10
# but the manifests might be created differently using new mixer.
###############################################################################

# update mixer to build version 30, which in our case is the +20
mixer versions update --mix-version 30
# update mixer.state to new format
sed -i 's/\(FORMAT\).*/\1 = "2"/' mixer.state
# Fully remove deleted bundles from groups.ini and mixbundles list. This will
# cause the deprecated bundles to be removed from the MoM entirely. This will
# not break users who had these bundles because the removed content in the +10
# caused the bundles to be dropped from client systems at that point.
for i in $(grep -lir "\[STATUS\]: Deprecated" upstream-bundles/ local-bundles/); do
	b=$(basename $i)
	mixer bundle remove $b; sed -i "/\[$b\]/d;/group=$b/d" update/groups.ini;
done
# link the +10 bundles to the +20 so we are building the update with the same
# underlying content. The only things that might change are the manifests
# (potentially the pack and full-file formats as well, though this is very
# rare).
cp -al update/image/20 update/image/30
# build an update as a minversion, this is the first build where the manifests
# identify as the new format
mixer build update --native --min-version 30
