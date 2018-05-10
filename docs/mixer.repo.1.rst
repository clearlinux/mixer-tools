==========
mixer.repo
==========

---------------------------------------------------------------
Perform various configuration actions on mixer RPM repositories
---------------------------------------------------------------

:Copyright: \(C) 2018 Intel Corporation, CC-BY-SA-3.0
:Manual section: 1


SYNOPSIS
========

``mixer repo [command]``


DESCRIPTION
===========

Perform various configuration actions on mixer RPM repositories. These RPM
repositories are used as content sources for mixer to build updates from.


OPTIONS
=======

In addition to the globally recognized ``mixer`` flags (see ``mixer``\(1) for
more details), the following options are recognized.

-  ``-c, --config {path}``

   The `path` to the configuration file to use.

-  ``-h, --help``

   Display subcommand help information and exit.


SUBCOMMANDS
===========

``add {name} {url}``

    Add the repo named `name` at the `url` url. In addition to the global
    options ``mixer repo add`` takes the following options.

``init``

    Initialize the DNF configuration file with the default `Clear` repository
    enabled.

``list``

    List all RPM repositories configured in the DNF configuration file used by
    mixer.

``remove {name}``

    Remove the repo `name` from the DNF configuration file used by mixer.

``set-url {name} {url}``

    Sets the URL for repo `name` to the provided `url`. If `name` does not exist
    the repo will be added to the configuration.


EXIT STATUS
===========

On success, 0 is returned. A non-zero return code indicates a failure.

SEE ALSO
--------

* ``mixer``\(1)
