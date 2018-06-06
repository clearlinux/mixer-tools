=====
mixin
=====

-------------------------
OS custom content manager
-------------------------

:Copyright: \(C) 2018 Intel Corporation, CC-BY-SA-3.0
:Manual section: 1


SYNOPSIS
========

``mixin [subcommand] <flags>``


DESCRIPTION
===========

``mixin``\(1) is a custom content manager that allows users to add custom local
or remote content to their client systems and still receive updates from their
upstream OS vendor.

A user can add remote RPM repositories or local RPMs and `mix` them into their
update stream. ``mixin`` runs ``mixer``\(1) to generate an update locally on the
client systems. The output metadata is then merged with the upstream metadata to
provide a single source of update for ``swupd``\(1) to perform updates.


SUBCOMMANDS
===========

``build``

    Build a mix from local or remote mix content existing/configured under
    `/usr/share/mix`. The output of this command can be used by ``swupd`` via
    the ``swupd update --migrate`` command.

``help``

    Print help text for any ``mixin`` subcommand.

``package add <package-name> [--build] [--bundle <name>]``

    Add RPM packages from remote or local repositories for use by mixer-swupd
    integration. This command performs a check to see if the added package
    exists in any of the configured repositories. The package is then added to
    a custom bundle named after the repo from which the package originated.

    Adding the optional `--build` flag will run ``mixin build`` after adding the
    package.

    Adding the option `--bundle <name>` flag will add the package to `name`
    instead of the repo name.

``repo``

    Add, list, remove, or edit RPM repositories to be used by ``mixin``. The DNF
    configuration that is modified is the configuration that exists in the
    `/usr/share/mix` directory. Commands that write to this configuration will
    require root permissions. This command supports the same options as the
    ``mixer repo`` command. See ``mixer.repo``\(1) for more information.


EXIT STATUS
===========

On success, 0 is returned. A non-zero return code indicates a failure.

SEE ALSO
--------

* ``mixer``\(1)
* ``mixer.repo``\(1)
* ``swupd``\(1)
* https://github.com/clearlinux/mixer-tools
* https://clearlinux.org/documentation/
