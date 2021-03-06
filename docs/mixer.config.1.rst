============
mixer.config
============

------------------------------------------
Perform configuration manipulation actions
------------------------------------------

:Copyright: \(C) 2018 Intel Corporation, CC-BY-SA-3.0
:Manual section: 1


SYNOPSIS
========

``mixer config [command]``


DESCRIPTION
===========

Perform various configuration manipulation actions on the mixer configuration
files.


OPTIONS
=======

In addition to the globally recognized ``mixer`` flags (see ``mixer``\(1) for
more details), the following options are recognized.

-  ``-h, --help``

   Display ``config`` help information and exit.


SUBCOMMANDS
===========

``convert``

    Convert an old config file to the new TOML format. The command will generate
    a backup file of the old config and will replace it with the converted one.
    Environment variables will not be expanded and the values will not be
    validated. In addition to the global options ``mixer config convert`` takes
    the following options.

    - ``-c, --config {path}``

      The `path` to the configuration file to convert.

    - ``-h, --help``

      Display ``config convert`` help and exit.

``validate``

    Parse a builder config file and display its properties. Properties
    containing environment variables will be expanded.  In addition to the
    global options ``mixer config validate`` takes the following options.

    - ``-c, --config {path}``

      The `path` to the configuration file to validate.

    - ``-h, --help``

      Display ``config validate`` help and exit.


EXIT STATUS
===========

On success, 0 is returned. A non-zero return code indicates a failure.

SEE ALSO
--------

* ``mixer``\(1)
