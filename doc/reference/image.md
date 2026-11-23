# Incus images

Operations Center provides Incus images to the Incus servers it manages. It
fetches images from image sources and keeps a local copy. It also holds images
that administrators upload. Operations Center serves all these images to Incus
as a single `simplestreams` remote.

This allows Incus servers in an air gapped environment to create instances.
Only Operations Center needs access to the image sources.

## Image sources

An image source is a server that provides Incus images in the `simplestreams`
format, for example `https://images.linuxcontainers.org`. Operations Center
reads `streams/v1/index.json` from the URL of the source. The index needs to
contain an entry with the data type `image-downloads`.

Each image source has the following properties:

| Property            | Description                                             | Example                              |
|:--------------------|:--------------------------------------------------------|:-------------------------------------|
| `name`              | Name of the image source                                | `linuxcontainers`                    |
| `url`               | URL of the `simplestreams` server                       | `https://images.linuxcontainers.org` |
| `filter_expression` | Selects the files to fetch, see [Filtering](#filtering) | `architecture == "amd64"`            |
| `last_updated`      | Time of the last change of the image source             | `2026-11-12T10:57:43Z`               |

The following rules apply:

* The name can only contain lower case letters, digits and underscores.
* The names `local` and `create` are reserved.
* The name and the URL are unique.
* The name cannot be changed after the image source is created.
* The filter expression cannot be empty. Use `true` to fetch every image of
  the source.

Removing an image source also removes all its images and their files.

### The image source `local`

The image source `local` always exists. It holds the images that administrators
upload, see [Upload images](#upload-images).

The image source `local` has no URL and no filter expression. It cannot be
changed, refreshed or removed.

## Refresh

A refresh aligns the images of an image source with the images the source
provides.

Operations Center refreshes all image sources at startup and then every 6
hours. On IncusOS, the first refresh starts 10 minutes after startup. A refresh
can also be triggered manually:

```shell
operations-center image incus refresh <source>
```

The manual refresh returns when the refresh is complete.

A refresh performs the following steps:

1. Fetch the list of images from the image source.
1. Apply the filter expression to each file.
1. Skip invalid images, for example images with an unsupported architecture.
   Operations Center removes the images it already has for such a name.
   Skip products of the source that map to the same image name. Images that
   Operations Center already has for such a name are kept, because the
   conflict might be temporary.
1. Remove the images, versions and files that the source no longer provides or
   that no longer match the filter expression.
1. Download the missing files. Operations Center checks the SHA256 checksum of
   each file, if the source provides one.
1. Store the images with the files that are on disk. The aliases and the
   description come from the image source.

A file that fails to download is not stored. The next refresh downloads it
again. Incus cannot use a version with missing files. So if a file of a new
version fails, Operations Center skips the whole version. A version that
Operations Center already has keeps the files it had before the refresh. If the
file `incus.tar.xz` of a version fails, Operations Center skips the whole
version.

Operations Center keeps exactly the versions that the image source provides
and that match the filter expression. There is no separate setting for the
number of versions to keep.

Before downloading the files of an image, Operations Center checks the free
disk space. At least 10% of the disk need to stay free. Otherwise, the refresh
stops downloading with an error. After 10 failed downloads, the refresh stops
downloading as well. In both cases, Operations Center still stores the images
with the files that are on disk.

If the automatic refresh fails, Operations Center shows the warning
"Refresh image sources failed". The next successful refresh removes the
warning. A failing image source does not stop the refresh of the other image
sources.

## Filtering

The filter expression of an image source selects the files that Operations
Center fetches. See [Filtering](filters) for more information on the syntax of
the filter expression.

Operations Center evaluates the filter expression once for each file of each
version of each image:

* A file is fetched, if the expression returns `true`.
* A version without any matching file is skipped.
* An image without any matching version is skipped.

If the filter expression changes, the next refresh removes the files that no
longer match. This also applies to files that are already downloaded.

The following properties are available in the filter expression:

| Property           | Description                                        | Example                                                      |
|:-------------------|:---------------------------------------------------|:-------------------------------------------------------------|
| `name`             | Name of the image, see [Images](#images)           | `debian:trixie:amd64:cloud`                                  |
| `operating_system` | Operating system, as provided by the image source  | `Debian`, `Alpine`, `Ubuntu`                                 |
| `release`          | Release of the operating system                    | `trixie`, `3.22`, `noble`                                    |
| `architecture`     | Architecture, see [Images](#images) for the values | `amd64`, `arm64`                                             |
| `variant`          | Variant of the image                               | `default`, `cloud`                                           |
| `version`          | Version of the image                               | `20261112_05:24`                                             |
| `filename`         | Name of the file                                   | `incus.tar.xz`, `root.squashfs`, `disk.qcow2`, `root.tar.xz` |
| `file_type`        | Type of the file                                   | `incus.tar.xz`, `squashfs`, `disk-kvm.img`, `root.tar.xz`    |
| `size`             | Size of the file in bytes                          | `123456789`                                                  |

The file types have the following meaning:

* `incus.tar.xz`: the metadata of the image. Incus needs it for every
  instance type.
* `squashfs`: the root file system for containers.
* `disk-kvm.img`: the disk for virtual machines.
* `root.tar.xz`: the root file system for containers, as a compressed tar
  archive.

The following rules apply:

* The filter expression needs to return a boolean value.
* Operations Center checks the filter expression, when an image source is
  created or changed. An unknown property is an error.
* An error during the evaluation of the filter expression fails the refresh.
* The comparison of text is case sensitive. Except for `name` and
  `architecture`, the values come from the image source. The name is in lower
  case and the architecture is mapped, see [Images](#images). Use
  `lower(operating_system) == "debian"` to ignore the case.
* The image source, the aliases and the description are not available in the
  filter expression.

```{note}
An empty filter expression of an update means "fetch everything". An image
source does not accept an empty filter expression. Use `true` instead.
```

## Images

An image is identified by its image source and its name. The name has the
format `os:release:architecture:variant` in lower case, for example
`debian:trixie:amd64:cloud`. The same name can exist in several image sources.

Operations Center supports the following architectures: `amd64`, `arm64`,
`armhf`, `loong64` and `riscv64`. It maps `x86_64` to `amd64`, `aarch64` to
`arm64` and `loongarch64` to `loong64`.

An image has one or more versions. The version starts with a date in the
format `yyyymmdd`, for example `20261112` or `20261112_05:24`.

A version can have the following files:

| File            | File type      | Description                                |
|:----------------|:---------------|:-------------------------------------------|
| `incus.tar.xz`  | `incus.tar.xz` | Metadata of the image, always present      |
| `root.squashfs` | `squashfs`     | Root file system for containers            |
| `disk.qcow2`    | `disk-kvm.img` | Disk for virtual machines                  |
| `root.tar.xz`   | `root.tar.xz`  | Root file system as compressed tar archive |

An image source can provide more file types, for example `lxd.tar.xz` or delta
files. Operations Center fetches every file that matches the filter
expression. Use `file_type in ["incus.tar.xz", "squashfs", "disk-kvm.img"]` to
fetch only the file types above.

Only the images of the image source `local` can be changed. Their description
and aliases can be edited. These images and their versions can be removed. The
images of other image sources follow the image source. Change the filter
expression of the image source to remove images, versions or files.

## Upload images

Administrators can upload images to the image source `local`. Each upload adds
one version of an image. There are two ways to provide the metadata of the
image.

### Upload with a metadata tarball

The metadata tarball is the file `incus.tar.xz`. Incus writes it with
`incus image export`. The metadata tarball needs to contain `metadata.yaml`
with the following properties:

| Property       | Description                     | Required                 |
|:---------------|:--------------------------------|:-------------------------|
| `os`           | Operating system                | Yes                      |
| `release`      | Release of the operating system | No, default is `current` |
| `architecture` | Architecture                    | Yes                      |
| `variant`      | Variant                         | No, default is `default` |
| `serial`       | Version of the image            | Yes                      |
| `description`  | Description of the image        | No                       |

The metadata tarball is the first argument. The image files follow:

```shell
operations-center image incus image add incus.tar.xz rootfs.squashfs
```

### Upload with attributes

Without a metadata tarball, the flags provide the metadata. Operations Center
then creates the metadata tarball. Every argument is an image file:

```shell
operations-center image incus image add --os debian --release trixie --arch amd64 --variant cloud --image-version 20261112 disk.qcow2
```

The flags `--os`, `--arch` and `--image-version` are required. The flag
`--release` defaults to `current`, and `--variant` defaults to `default`.

### Image files

Operations Center detects the type of each image file from its content. The
name of the uploaded file does not matter:

* A `squashfs` image is stored as `root.squashfs`.
* A `qcow2` disk is stored as `disk.qcow2`.
* An `xz` compressed tar archive is stored as `root.tar.xz`.

Each file type can be uploaded once per version. Other files are rejected.

A new image gets the alias `<os>/<release>/<variant>`. Operations Center
rejects the upload of a version that already exists. Remove the version
first to replace it.

## Use the images in Incus

Operations Center serves the images of all image sources at the path
`/incus-images`. This path needs no authentication. Add Operations Center as
a remote in Incus:

```shell
incus remote add operations-center https://<operations-center-address>/incus-images --protocol simplestreams
incus launch operations-center:debian/trixie/cloud my-instance
```

The Incus server needs to trust the TLS certificate of Operations Center. For
example, use a certificate from [ACME](settings.md#acme). For a private
certificate authority (CA), add the CA certificate to the IncusOS servers as
described in [ACME server with a private CA](settings.md#acme-server-with-a-private-ca).

```{note}
The Incus command line tool caches the image list of a `simplestreams` remote
for up to one hour. New images might therefore not show up immediately.
```

Images of different image sources can have the same alias. Incus would pick
one of them without a warning. Operations Center therefore publishes each
alias only once per architecture. The first image with the alias keeps it, in
the following order:

1. Images of the image source `local`.
1. Images of the other image sources, in the order the image sources were
   created.

An image that loses an alias is still available by its fingerprint.

Operations Center does not publish images without versions. Such an image
keeps none of its aliases, so an image of another source can use them.

## Command line and web interface

The command `operations-center image incus` manages the image sources:

| Command                                    | Description                              |
|:-------------------------------------------|:-----------------------------------------|
| `list`                                     | List the image sources                   |
| `show <source>`                            | Show an image source                     |
| `add <source> <url> --filter <expression>` | Add an image source                      |
| `edit <source>`                            | Change the URL and the filter expression |
| `remove <source>`                          | Remove an image source and its images    |
| `refresh <source>`                         | Refresh an image source                  |

The command `operations-center image incus image` manages the images of an
image source:

| Command                                                       | Description                                                                    |
|:--------------------------------------------------------------|:-------------------------------------------------------------------------------|
| `list <source>`                                               | List the images of an image source                                             |
| `show <source> <name>`                                        | Show an image and its versions                                                 |
| `add <metadata tarball> <file> [<file> ...]`                  | Upload a version to the image source `local`                                   |
| `edit <source> <name>`                                        | Change the description and the aliases of an image in the image source `local` |
| `remove <source> <name>`                                      | Remove an image from the image source `local`                                  |
| `remove-version <source> <name> <version>`                    | Remove a version of an image from the image source `local`                     |
| `file <source> <name> <version> <filename> <target-filename>` | Download a file of an image                                                    |

In the web interface, the entry "Images" in the navigation shows the image
sources. From there, the images of each image source are available. Images
can be uploaded to the image source `local`.

## Backup

A backup includes the images only with `--with-images`. Without the images, a
restore removes the images on the system. Operations Center fetches the images
of the image sources again. The uploaded images need to be uploaded again. See
[Backup and restore](backup) for more information.

The log component `image` covers the image service. The log component
`daemon.task.refresh_image_sources` covers the automatic refresh. See
[Settings](settings) for more information on log components.
