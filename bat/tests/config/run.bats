#!/usr/bin/env bats

# shared test functions
load ../../lib/mixerlib

setup() {
    global_setup
}

@test "test config setter and getter" {
    mixer-init-stripped-down $CLRVER 10

    # Setter and getter on builder.conf
    mixer config set Swupd.BUNDLE test-bundle
    bundle=$(mixer config get Swupd.BUNDLE)
    test $bundle = "test-bundle"

    # Setter and Getter on mixer.state
    mixer config set Mix.FORMAT 99
    format=$(mixer config get Mix.FORMAT)
    test $format -eq 99
}
# vi: ft=sh ts=8 sw=2 sts=2 et tw=80
