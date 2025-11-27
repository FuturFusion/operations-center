# Cluster

Clusters represent a group of Incus servers running on top of IncusOS, that
allow to spread workloads across multiple servers.

Operations Center allows to provision clusters from registered [servers](server.md).

Provisioning of a cluster can be done through two slightly different approaches:

* [One off clustering](#one-off-clustering)
* [Template based clustering](#template-based-clustering)

In both cases, the administrator needs to provide the
[service configuration](#service-configuration) and the
[application configuration](#application-configuration).

Once one or many servers are clustered, Operations Center will automatically
keep track of their [inventory](inventory.md).

## Service Configuration

IncusOS system services are optional system-wide features, typically used to
integrate with an external system like storage or networking. The complete
list of services can be found in the
[IncusOS services documentation](https://linuxcontainers.org/incus-os/docs/main/reference/services/).

During clustering, service configuration is applied on each server.
The clustering process accepts a single configuration file (YAML or JSON)
containing the configuration for all services, where each
[service name](https://linuxcontainers.org/incus-os/docs/main/reference/api/#/services/services_get)
is a top-level key with the respective configuration underneath it.

Example with LVM and nvme service:

```yaml
---
lvm:
  enabled: true
  # System ID is automatically determined by Operations Center during clustering.
  # system_id: 0
nvme:
  enabled: true
  targets:
    - transport: tcp
      address: 192.168.1.100
      port: 8009
```

## Application Configuration

The application configuration provided during clustering follows the same format
as the preseed configuration used by Incus for
[non-interactive configuration](https://linuxcontainers.org/incus/docs/main/howto/initialize/#initialize-preseed)
(see [InitLocalPreseed](https://github.com/lxc/incus/blob/main/shared/api/init.go)
struct definition for full details).

Example:

```yaml
---
config:
  user.ui.title: "My wonderful cluster"
certificates:
  - type: client
    name: my-client-cert
    description: "Client certificate for accessing the cluster"
    certificate: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

## One Off Clustering

One off clustering takes a service configuration file, an application
configuration file and the list of to be clustered servers as arguments.

## Template Based Clustering

Template based clustering uses a [cluster-template](cluster-template.md),
a file containing key-value pairs for the defined variables in the
cluster-template and the list of to be clustered servers as arguments.

The file containing the variables has the following format (YAML):

```yaml
---
SOME_VARIABLE: "the value"
A_BOOLEAN_VARIABLE: true
A_NUMERIC_VARIABLE: 42
```
