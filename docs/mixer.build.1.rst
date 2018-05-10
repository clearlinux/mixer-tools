===========
mixer.build
===========

-----------------------------------
Build varioius pieces of OS content
-----------------------------------

:Copyright: \(C) 2018 Intel Corporation, CC-BY-SA-3.0
:Manual section: 1


SYNOPSIS
========

``mixer build [command] [flags]``


DESCRIPTION
===========

Builds various pieces of OS content and output update metadata to
`<mixer/workspace>/update/www/<version>/`. This metadata can be published
directly to a web-server and consumed by client update systems via the
``swupd``\(1) update tool. All steps can be run at once using ``build all`` or
the steps can be run individually using the subcommands.


OPTIONS
=======

In addition to the globally recognized ``mixer`` flags (see ``mixer``\(1) for
more details), the following options are recognized across all ``build``
subcommands.

-  ``--bundle-workers``

   Number of parallel workers when building bundles, passing 0 or omitting this
   flag defaults the number of workers to the number of CPUs on the system.

-  ``--delta-workers``

   Number of parallel workers when creating deltas, passing 0 or omitting this
   flag defaults the number of workers to the number of CPUs on the system.

-  ``--fullfile-workers``

   Number of parallel workers when creating fullfiles, passing 0 or omitting this
   flag defaults the number of workers to the number of CPUs on the system.

-  ``-h, --help``

   Display ``build`` help information and exit.


SUBCOMMANDS
===========

``all``

    Build all content for the mix. Implicitly runs ``build bundles`` followed by
    ``build update``. In addition to the global options ``mixer build all``
    takes the following options.

    - ``-c, --config {path}``

      Optionally tell ``mixer`` to use the configuration file at `path`. Uses
      the default `builder.conf` in the mixer workspace if this option is not
      provided.

    - ``--format {number}``

      Supply the format number to use for the build.

    - ``-h, --help``

      Display ``build all`` help information and exit.

    - ``--increment``

      Automatically increment the mix version post build.

    - ``--min-version {version}``

      Supply minimum version for ``mixer`` to use old content from. This option
      tells ``mixer`` to regenerate all mix content starting from a certain
      version. ``mixer`` will not use any OS content from a version older than
      the min-version passed here.

    - ``--no-publish``

      Do not update the LAST_VER file after the update. Any ``swupd`` client
      configured to update from the mix will not be made aware of the new mix
      version and will therefore not attempt an update.

   - ``--no-signing``

     Do not generate a certificate and do not sign the Manifest.MoM

   - ``--prefix {path}``

     Supply the `path` to the file system where the ``swupd`` binaries live.

``bundles``

    Build the bundles for your mix. This is done by extracting dependency
    information and file lists for each package in each bundle definition for the
    mix. In addition to the global options ``mixer build bundles`` takes the
    following options.

    - ``-c, --config {path}``

      Optionally tell ``mixer`` to use the configuration file at `path`. Uses
      the default `builder.conf` in the mixer workspace if this option is not
      provided.

    - ``-h, --help``

      Display ``build bundles`` help information and exit.

   - ``--no-signing``

     Do not generate a certificate and do not sign the Manifest.MoM

``delta-packs``

    Build packs to optimize ``swupd update``\s between versions. When a
    ``swupd`` client updates a bundle it looks for a pack file from its current
    version to the new version. If available ``swupd`` will download and apply
    the pack content to the file system. Delta packs contain binary diff files
    that describe changes between updates whenever possible and full files only
    when necessary. Because of this delta packs are a significant performance
    optimization for client updates. Because the client can fall back to full
    files if a pack is not available, delta packs are not necessary for a
    functional update. In addition to the global options ``mixer build
    delta-packs`` takes the following options.

    - ``-c, --config {path}``

      Optionally tell ``mixer`` to use the configuration file at `path`. Uses
      the default `builder.conf` in the mixer workspace if this option is not
      provided.

    - ``--from {version}``

      Generate packs from the specified `version`.

    - ``-h, --help``

      Display ``build bundles`` help information and exit.

    - ``--previous-versions {number}``

      Generate packs for `number` of previous versions.

    - ``--report``

      Report reason each file in the `to` manifest was packed in the delta pack
      or not.

    - ``--to {version}``

      Generate packs targeting a specific `to` `version`.

``image``

    Build an image from the mix content. In addition to the global options
    ``mixer build image`` takes the following options.

    - ``-c, --config {path}``

      Optionally tell ``mixer`` to use the configuration file at `path`. Uses
      the default `builder.conf` in the mixer workspace if this option is not
      provided.

    - ``--format {number}``

      Supply the format `number` used for the mix.

    - ``-h, --help``

      Display ``build bundles`` help information and exit.

    - ``--template {path}``

      Provide the `path` to the image template file to use.

``update``

    Build the update content for the mix. This command builds the actual update
    metadata (manifests) and content (full files and zero packs) necessary for
    ``swupd`` to perform updates on client systems. ``update`` relies on the
    output of ``build bundles`` as the input for this step and expects the
    output of ``build bundles`` to exist in the
    `<mixer/workspace>/update/image/<version>` directory. In addition to the
    global options ``mixer build update`` takes the following options.

    - ``-c, --config {path}``

      Optionally tell ``mixer`` to use the configuration file at `path`. Uses
      the default `builder.conf` in the mixer workspace if this option is not
      provided.

    - ``--format {number}``

      Supply the format `number` used for the mix.

    - ``-h, --help``

      Display ``build bundles`` help information and exit.

    - ``--increment``

      Automatically increment the mix version post build.

    - ``--min-version {version}``

      Supply minimum version for ``mixer`` to use old content from. This option
      tells ``mixer`` to regenerate all mix content starting from a certain
      version. ``mixer`` will not use any OS content from a version older than
      the min-version passed here.

    - ``--no-publish``

      Do not update the LAST_VER file after the update. Any ``swupd`` client
      configured to update from the mix will not be made aware of the new mix
      version and will therefore not attempt an update.

   - ``--no-signing``

     Do not generate a certificate and do not sign the Manifest.MoM

   - ``--prefix {path}``

     Supply the `path` to the file system where the ``swupd`` binaries live.


EXIT STATUS
===========

On success, 0 is returned. A non-zero return code indicates a failure.

SEE ALSO
--------

* ``mixer``\(1)
* ``swupd``\(1)
