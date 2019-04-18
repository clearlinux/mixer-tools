#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Perform format bump using individual build format-bump <new/old>" {
  upstreamver=$(get-last-format-boundary)
  mixer-init-stripped-down $upstreamver 10
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  # create deprecated bundle
  cat << EOF > local-bundles/editors
# [TITLE]: editors
# [DESCRIPTION]: Run popular terminal text editors
# [STATUS]: Deprecated
# [CAPABILITIES]:
# [MAINTAINER]: Test Master <email@example.com>
EOF
  mixer bundle add editors

  #Create initial version in the old format
  mixer-build-bundles > $LOGDIR/build_bundles.log

  mixer-build-update > $LOGDIR/build_update.log

  #Build +10
  format=$((format+1))
  mixer-build-format-bump-old $format > $LOGDIR/build_formatbump_old.log

  #Build +20
  mixer-build-format-bump-new $format > $LOGDIR/build_formatbump_new.log

  #check if LAST_VER is set to +20
  test $(< update/image/LAST_VER) -eq 30

  #check PREVIOUS_MIX_VERSION matches LAST_VER
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 30

  #check if deprecated bundle is marked deleted in +10
  awk '
  $4 == "editors" {
    found = 1; exit $3 != 20
  }
  END {
  if (found != 1)
    exit 1
  }' update/www/20/Manifest.MoM

  #editors bundle should have all files marked as deleted in +10 (0's)
  # NOTE* This has to be updated if the number of fields in a manifest
  # file entry changes from 4 which it currently is at.
  awk 'NF == 4 { if ($2 !~ /^0{64}$/) exit 1 }' update/www/20/Manifest.editors


  #editors bundle should not exist in +20
  [ ! -f update/www/30/Manifest.editors ]


  #editors bundle should not exist in +20 MoM
  ! grep "editors" update/www/30/Manifest.MoM

  #check if +10 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/20/Manifest.MoM

  #check if +20 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/30/Manifest.MoM

  #check if +20 format is set to formatN
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/30/Manifest.MoM

  format=$((format-1))
  #check if +10 format is set to formatN+1
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/20/Manifest.MoM
}

@test "Perform format bump using top level command" {
  upstreamver=$(get-last-format-boundary)
  mixer-init-stripped-down $upstreamver 10
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  # create deprecated bundle
  cat << EOF > local-bundles/editors
# [TITLE]: editors
# [DESCRIPTION]: Run popular terminal text editors
# [STATUS]: Deprecated
# [CAPABILITIES]:
# [MAINTAINER]: Test Master <email@example.com>
EOF
  mixer bundle add editors

  #Create initial version in the old format
  mixer-build-bundles > $LOGDIR/build_bundles.log

  mixer-build-update > $LOGDIR/build_update.log

  #Build +10
  format=$((format+1))
  mixer-build-format-bump $format > $LOGDIR/build_formatbump_one_step.log

  #check if LAST_VER is set to +20
  test $(< update/image/LAST_VER) -eq 30

  #check PREVIOUS_MIX_VERSION matches LAST_VER
  test $(sed -n 's/[ ]*PREVIOUS_MIX_VERSION[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 30

  #check if deprecated bundle is marked deleted in +10
  awk '
  $4 == "editors" {
    found = 1; exit $3 != 20
  }
  END {
  if (found != 1)
    exit 1
  }' update/www/20/Manifest.MoM

  #editors bundle should have all files marked as deleted in +10 (0's)
  # NOTE* This has to be updated if the number of fields in a manifest
  # file entry changes from 4 which it currently is at.
  awk 'NF == 4 { if ($2 !~ /^0{64}$/) exit 1 }' update/www/20/Manifest.editors



  #editors bundle should not exist in +20
  [ ! -f update/www/30/Manifest.editors ]

  #editors bundle should not exist in +20 MoM
  ! grep "editors" update/www/30/Manifest.MoM

  #check if +10 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/20/Manifest.MoM

  #check if +20 previous version is set to +0
  awk '$1 == "previous:" { exit $2 != 10}' update/www/30/Manifest.MoM

  #check if +20 format is set to formatN
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/30/Manifest.MoM

  format=$((format-1))
  #check if +10 format is set to formatN+1
  awk -v f=$format '$1 == "MANIFEST" {exit $2 != f}' update/www/20/Manifest.MoM
}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
