#!/bin/bash

set -e

# No defaults set for now; we haven't decided on BKMs yet for adding RPMs to a
# mix. For now, everything is done manually (create dir; add RPM(s) to it).
REPODIR=
RPMDIR=

function usage() {
  echo -n "Usage: $0"
  echo -n " [-h | --help]"
  echo -n " [-r <dir> | --rpmdir <dir>]"
  echo -n " [-d <dir> | --repodir <dir>]"
  echo
  echo
  echo -n "The -r value specifies the directory containing binary RPMs,"
  echo
  echo -n "and the -d value is the directory to use for the local repo."
  echo
}

while [ "$1" != "" ]; do
  case "$1" in
    -r | --rpmdir )
      shift; RPMDIR="$1"
      ;;
    -d | --repodir )
      shift; REPODIR="$1"
      ;;
    -h | --help )
      usage; exit 1
      ;;
    * )
      usage; exit 1
      ;;
  esac
  shift
done

if [ ! -d "$RPMDIR" ]; then
  echo "Path to directory containing must be specified with the -r option"
  exit 1
fi

if [ ! -d "$REPODIR" ]; then
  echo "Path to local RPM repository must be specified with the -d option"
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

# later scripts need to know to repo location
echo "$REPODIR" > "$PWD/.mixer-repopath"

exit 0

# vi: ts=8 sw=2 sts=2 et tw=80
