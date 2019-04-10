#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Customize os-release file" {

  mixer init > /dev/null

  #Use modified config which has customized os-release file
  cp files/my-builder.conf builder.conf
  sed -i "s:/home/clr/mix:$PWD:g" builder.conf

  #Mixer build bundle
  mixer-build-bundles > $LOGDIR/build_bundles.log

  #Compare the diff between customized os-release file and output of mixer build
  #bundle. The only difference should be VERSION_ID 
  run bash -c "diff files/my-os-release update/image/10/full/usr/lib/os-release | wc -l "
  [ "$output" -eq  4 ]

  run bash -c "diff files/my-os-release update/image/10/full/usr/lib/os-release"
  [[ ${lines[1]} =~ "VERSION_ID" ]]
  [[ ${lines[3]} =~ "VERSION_ID" ]]
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
