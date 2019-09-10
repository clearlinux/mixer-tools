===========
mixer.reset
===========

------------------------------------------
Reset mixer to a given or previous version
------------------------------------------

:Copyright: \(C) 2019 Intel Corporation, CC-BY-SA-3.0
:Manual section: 1


SYNOPSIS
========

``mixer reset [flags]``


DESCRIPTION
===========

Reverts a mix state to the end of the last build or to the end
of a given version build if one is provided. By default, the value
of PREVIOUS_MIX_VERSION in mixer.state will be used to define the
last build. This command can be used to roll back the mixer state
in case of a build failure or in case the user wants to roll back
to a previous version.

OPTIONS
=======

In addition to the globally recognized ``mixer`` flags (see ``mixer``\(1) for
more details), the following options are recognized.

-  ``--to``

   Reverts the mix to the version provided by the flag

-  ``--clean``

   Delete all files associated with versions that are bigger than the one provided.

-  ``-h, --help``

   Display ``reset`` help information and exit.


EXIT STATUS
===========

On success, 0 is returned. A non-zero return code indicates a failure.

SEE ALSO
--------

* ``mixer``\(1)
