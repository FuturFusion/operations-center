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

### Public Token Seed

A token seed configuration can be made **public**, which allows fetching of
pre-seeded installation images without authentication.
