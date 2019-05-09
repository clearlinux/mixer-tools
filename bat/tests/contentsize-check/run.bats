#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup

  current_ver=$(get-current-version)
  mixer-init-stripped-down "$current_ver" 10
  mixer-build-all
}

@test "Compare calculated contentsize with actual" {
  manifestPrefix=update/www/10/Manifest.
  manifests=$(sed -n 's/^[M,I]\..*\t//p' "${manifestPrefix}"MoM)

  while read -r manifest; do
    chroot=update/image/10/full/

    # Only files and links are included in the contentsize. Directories
    # can be installed with variable sizes, so they are omitted from the
    # contentsize calculation.
    files=$(sed -n 's/^[F,L].*\t//p' "${manifestPrefix}${manifest}")

    # Add the content sizes of the files and links
    total=0
    while read -r f; do
      if [ -z "$f" ]; then
        continue
      fi
      size=$(stat -c "%s" "${chroot}${f}")
      total=$((total + size))
    done <<< "$files"

    manifestSize=$(sed -n 's/^contentsize:\t//p' "${manifestPrefix}${manifest}")

    # Compare the calculated contentsize against the manifest's contentsize
    test "$total" -eq "$manifestSize" 
  done <<< "$manifests"
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
