# Backup and restore

Operations Center can back up its state into a `gzip` compressed tar archive and
restore it again.

## Content of a backup

A backup holds:

* the configuration (`config.yml`)
* the server and client certificates and keys
* a consistent snapshot of the database
* the cluster artifacts
* the images served by Operations Center, only if requested
* the cached update files, only if requested

The first entry of the archive is the manifest `backup.yaml`. It records the
format version of the backup, the version of Operations Center, which created
it, the time of creation and if the images and the cached update files are
included.

The inventory is not part of a backup. It is synced again from the clusters
after a restore.

```{note}
A backup contains private keys and credentials. Store it safely.
```

## Create a backup

```
operations-center system backup operations-center-backup.tar.gz
```

The images and the cached update files are only included on request:

* `--with-images` includes the images
* `--with-updates` includes the cached update files
* `--complete` includes both

Without the cached update files, a restore keeps the cached update files, which
are known to the restored database, and the missing updates are fetched again
from the update source.

Without the images, the backup holds the image sources but no images. A restore
removes the images present on the system, they are fetched again from the image
sources. Uploaded images have to be uploaded again.

## Restore a backup

```
operations-center system restore operations-center-backup.tar.gz
```

The backup is validated first. A backup without manifest, with a format version
not supported by this version of Operations Center or created by a newer
version of Operations Center is rejected. A backup of an older version is
upgraded on restore.

Operations Center then restarts and replaces its state with the one from the
backup. If Operations Center does not start successfully with the
restored state, it puts back the state from before the restore on the next
start.

A restore is refused, while a server is being deployed, updated, evacuated or
restored or while a cluster update is in progress. Operations, which have been
in progress, when the backup has been created, are aborted after the restore:

* Cluster updates are aborted.
* Server deployments are canceled, the servers are left untouched.

Backup and restore require the `admin` role on the server object.
