# Update

Operations Center acts as the update hub for IncusOS servers. IncusOS servers
contact Operations Center to look for newer software and download updates if
available. This allows deployments to easily operate in an air gaped environment
by only having one place to push new update files to.

Operations Center handles the central IncusOS update management. In this role,
Operations Center keeps the relevant updates available locally.

Operations Center has a single source configured in the config file. If
Operations Center does have internet access, it checks the registered update
source on a recurring schedule (by default hourly) for new updates and downloads
them to the local cache.

It is also possible to operate Operations Center in air gapped environments,
where the updates are provided manually by the administrators.

## Filtering

The updates, which should be downloaded and be made available for the managed
IncusOS instances, can be filtered based on various criteria.

See [filtering](filters) for more information on the syntax of the filtering
expression.

### Update level filters

Update level filters are configured using the `filter_expression` config key
in the [Update settings](settings.md#update-settings).

They allow filtering of updates based on their metadata.

| Property            | Description                                  | Example                                     |
| :---                | :---                                         | :---                                        |
| `upstream_channels` | Upstream channels, the update is part of     | `stable`, `testing`, `daily`                |
| `origin`            | Source the update originates from            | `linuxcontainers.org`                       |
| `published_at`      | Timestamp when the update has been published | `2025-11-21T22:30:02.515408725Z`            |
| `severity`          | Severity of the update                       | `none`, `low`, `medium`, `high`, `critical` |
| `uuid`              | Unique identifier of the update              | `123e4567-e89b-12d3-a456-426614174000`      |
| `version`           | Version of the update                        | `202511201340`                              |

The following properties are available during filtering as well, but they are
not really useful as filter criteria and are only listed here for completeness.

* `changelog`: Content of the changelog
* `update_status`: state from Operations Center point of view, if the update is `ready` or `pending`; when the filtering is applied, the updates are never `ready`
* `url`: Source URL of the update

### File level filters

File level filters are configured using the `file_filter_expression` config key
in the [Update settings](settings.md#update-settings).

They allow filtering of update files based on their metadata.

| Property       | Description                             | Example                                                                                                                                                                      |
| :---           | :---                                    | :---                                                                                                                                                                         |
| `architecture` | Architecture the update file applies to | `x86_64`, `aarch64`                                                                                                                                                          |
| `component`    | Component of the update file            | `os`, `incus`, `incus-ceph`, `incus-linstor`, `operations-center`, `migration-manger`, `debug`                                                                               |
| `filename`     | Name of the update file                 | `x86_64/IncusOS_202511201340.efi.gz`                                                                                                                                         |
| `sha256`       | SHA256 checksum of the update file      | `26705d5d610a8f450d92e073ac0f691197c16c8fbcbd0a3cf71e13fea93f54b2`                                                                                                           |
| `size`         | Size of the update file in bytes        | `12345678`                                                                                                                                                                   |
| `type`         | Type of the update file                 | `image-raw`, `image-iso`, `image-manifest`, `changelog`, `update-efi`, `update-usr`, `update-usr-verity`, `update-usr-verity-signature`, `udpate-secure-boot`, `application` |
