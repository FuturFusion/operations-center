# Token

Tokens are used to register new IncusOS servers with Operations Center. The
tokens are time bound by an expiration time and usage count limited by a
maximum usage count.

Tokens are created by an administrator in Operations Center.

Operations Center allows to then generated pre-seeded installation images
(ISO and raw) for easy bootstrapping of new IncusOS servers.
The newly installed IncusOS servers will then use the token to self-register
with Operations Center.

## Token Seed

For each token, zero to many named seed configurations can be created. A seed
configuration may contain seed configuration for installation, network settings,
applications to be installed as well as seed configuration for the applications
themselves.

The details about the installation seed is documented in
[IncusOS Installation Seed](https://linuxcontainers.org/incus-os/docs/main/reference/seed/).

The seed configuration is only accepted, if it exclusively contains fields,
which are part of the respective seed definition. Unknown fields are rejected.

The seed configuration for Incus (`incus.yaml`) and for updates
(`update.yaml`) is managed by Operations Center itself and is therefore
rejected, if provided as part of a seed configuration.

### Token Seed Name

The name of a token seed is unique for each token. The name is part of the URL
and of the file name of the pre-seeded installation images. For this reason,
the name must follow these rules:

- The name starts with a letter or a digit.
- The name only contains letters (`a` to `z`, `A` to `Z`), digits, `.`, `_`
  and `-`.
- The name has a maximum length of 63 characters.

A token seed created before these rules existed keeps its name. Such a token
seed must be renamed before it can be updated.

A token seed can be renamed:

```shell
operations-center provisioning token seed rename <token-uuid> <name> <new-name>
```

The URL of the pre-seeded installation images changes with the name.

### Changing a Token Seed in Use

A server deployment uses a token seed until the deployment is finished. During
this time, Operations Center rejects the following operations on the token seed:

- Update, if the seed configuration or the public setting changes. A change of
  the description is always accepted.
- Rename.
- Remove.

Wait for the server deployments to finish or cancel them. Alternatively, use the
`--force` flag of the command line client or the `force` query parameter of the
REST API to apply the operation anyway.

```{warning}
A forced operation can break the server deployments, which use the token seed.
```

### Seed Configuration (`pre-seed.yaml`)

The seed configuration has the following structure:

```yaml
applications:
  version: "1"
  applications:
    - name: incus
    - name: debug
# install:
# ...
# migration_manager:
# ...
network:
  interfaces:
    - name: enp5s0
      hwaddr: enp5s0
      required_for_online: both
      addresses:
      - dhcp4
      - dhcp6
      - slaac
# ...
# operations_center:
# ...
```

### Public Token Seed

A token seed configuration can be made **public**, which allows fetching of
pre-seeded installation images without authentication.

Operations Center caches the pre-seeded installation images of a token seed.
Operations Center removes these cached images, when the token seed is renamed,
removed or changed from public to not public. After such a change, the images
are no longer available without authentication.
