#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
  global_setup
}

@test "Convert to latest config" {
  mixer init > /dev/null

  #Backup current config
  cp builder.conf current.conf

  for f in "configs"/*
  do
    cp $f builder.conf
    sed -i "s:/home/clr/mix:$PWD:g" builder.conf

    mixer config validate > /dev/null

    diff builder.conf current.conf
  done

}

@test "Test format transfer" {
  mixer init > /dev/null

  cp configs/nover_builder.conf builder.conf

  rm mixer.state

  mixer config validate > /dev/null

  #check if format was transfered from builder.conf to mixer.state
  test $(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq 1
}

@test "Test format keep" {
  mixer init > /dev/null

  #Save initial format
  format=$(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state)

  cp configs/nover_builder.conf builder.conf

  mixer config validate > /dev/null

  #check if format has been preserved
  test $(sed -n 's/[ ]*FORMAT[ ="]*\([0-9]\+\)[ "]*/\1/p' mixer.state) -eq $format
}

# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
