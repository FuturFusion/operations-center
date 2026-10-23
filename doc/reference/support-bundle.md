# Support bundle

A support bundle is a zip archive with the diagnostic data of an affected
resource, which is one of:

* a cluster
* a project in a cluster
* an instance in a project of a cluster
* a server

## Content of a support bundle

The data is collected live from Incus and IncusOS. A part of the data, which can
not be collected, e.g. because a server is not reachable, is replaced by a file
with the suffix `.error.txt`, which holds the error. The remaining data is still
included.

All files are placed in a directory named after the bundle:

| Path                                    | Content                                                                                      |
|:----------------------------------------|:---------------------------------------------------------------------------------------------|
| `manifest.json`                         | Request, version of Operations Center, time of creation and the status of every part         |
| `operations-center/`                    | Version and the records of the cluster and its servers in Operations Center                  |
| `cluster/`                              | Incus server information, cluster, cluster members with their state and warnings             |
| `cluster/*.json`                        | Projects, profiles, networks, network ACLs, storage pools and volumes, images and instances  |
| `projects/<project>/`                   | Project, its state, instances, profiles, networks, storage volumes and images                |
| `instances/<project>/<instance>/`       | Instance, snapshots, attached storage volumes and networks, instance logs and console log    |
| `servers/<server>/incus/`               | Incus server information and resources of the server                                         |
| `servers/<server>/os/`                  | IncusOS version, system configuration and state and the installed applications               |
| `servers/<server>/os/debug/`            | Journal of the current boot, kernel modules, processes and the logs of the Incus application |

What is included depends on the affected resource:

* **Cluster**: the cluster summary, the complete environment of the cluster and
  the data of all servers of the cluster.
* **Project**: the cluster summary, the resources of the project and the data of
  the servers hosting the instances of the project.
* **Instance**: the cluster summary, the instance and the data of the server
  hosting the instance.
* **Server**: the data of the server and, if the server is part of a cluster,
  the cluster summary.

The journal is limited to the latest 5000 entries of the current boot.

```{note}
Secrets in the configuration, e.g. passwords, tokens, private keys, recovery
keys and `cloud-init` user data, are replaced by `[REDACTED]`. The journal, the
process list and the logs are included as is and might still contain sensitive
data. Review a support bundle before sharing it.
```

## Create a support bundle

```
operations-center support-bundle cluster <cluster>
operations-center support-bundle project <cluster> <project>
operations-center support-bundle instance <cluster> <instance> [--project <project>]
operations-center support-bundle server <server>
```

The bundle is written to the file given by `--output` or to a file in the
current directory named after the affected resource and the time of creation.

In the UI, a support bundle is created with the support bundle action of a
cluster, a server, a project or an instance.

Collecting the data can take several minutes, in particular for a cluster with
many servers.

Creating a support bundle requires the `can_debug` entitlement on the server.
The `admin` relation grants this entitlement.
