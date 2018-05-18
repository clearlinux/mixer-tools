# Adding Custom Content from Build Root

When generating update content based on an upstream mix (such as upstream Clear
Linux), it is possible to provide custom bundles (and custom content) without
using RPMs as input and instead providing a full build root of the content after
the normal bundle building stage. There are a few steps that need to be
observed.

First, initialize your workspace and build your bundles as usual. This workflow
will assume a build based on an upstream mix.

```bash
mixer init --clear-version <version> --mix-version <mix-version>
mixer bundle add --all-upstream  # do not add your custom bundle
sudo mixer build bundles
```

At this point mixer has created several `bundle-info` files in your
`<ws>/update/image/<mix-version>/` directory. These files define the package and
file lists for each bundle that was added in the `mixer bundle add
--all-upstream` step above. Your custom bundle did not have a `bundle-info` file
created for it.

Now we will create your custom bundle. This tutorial assumes you have the build
root containing your custom content on your system already. How you generate
this is up to you.

Copy your custom content into the `<ws>/update/image/<mix-version>`

```bash
sudo mkdir <ws>/update/image/<mix-version>/<bundle-name>
sudo cp -a <path/to/custom/build/root> <ws>/update/image/<mix-version>/<bundle-name>
```

**WARNING:** it is possible to add files that conflict with files provided by
upstream. Take care to not introduce conflicting files. A conflicting file is a
file with the same filename as another file but with different contents,
permissions, or ownership. **Introducing a conflicting file can break client
updates.** Before adding your custom content, check for conflicting file names
in the `<ws>/image/<mix-version>/full` build root. This build root contains all
files in the mix. A common way to avoid file conflicts is to namespace your
content under a specific directory, such as `/<bundle-name>/*`.

An example of a conflicting file would be if your content provided a custom
`/usr/bin/yum` but the file exists in
`<ws>/image/<mix-version>/full/usr/bin/yum` with different content or
permissions.

Now update the `<ws>/update/groups.ini` file to add your custom bundle
(bundle-name) to your bundle list. At the bottom of that file add a new section.

```toml
[<bundle-name>]
group=<bundle-name>
```

Replace \<bundle-name\> with the name of your custom bundle. Remove the \<\>
brackets.

You can now continue building your update.

```bash
sudo mixer build update
```

Mixer will build an update including your custom bundle and custom content you
provided in the build root.
