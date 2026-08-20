# The cluster does already trust the following client certificates, e.g. because
# they have been applied with the seed config of the servers. They are therefore
# imported into the Terraform state instead of being added to the cluster.

resource "incus_certificate" "oc-trusted-b4e08ef4c7fb" {
  name        = "oc-trusted-b4e08ef4c7fb"
  description = "Client trusted by Operations Center"
  restricted  = false
  type        = "client"

  projects = []

  certificate = <<EOT
-----BEGIN CERTIFICATE-----
MIIB4zCCAWqgAwIBAgIUZT1U82k44TuXkKcq1jdXYxaZDDgwCgYIKoZIzj0EAwMw
ODEZMBcGA1UECgwQTGludXggQ29udGFpbmVyczEbMBkGA1UEAwwSdXNlckBob3N0
LnNvbWUudGxkMB4XDTI2MDgxMTA2MTgyOFoXDTM2MDgwODA2MTgyOFowODEZMBcG
A1UECgwQTGludXggQ29udGFpbmVyczEbMBkGA1UEAwwSdXNlckBob3N0LnNvbWUu
dGxkMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEW9cy2pfFn/i3XdgNGl5WqLkthPHS
4x5zQubllNA66BAcV23vVqJ0xXMfKctlVgrU0NRSNEn5mX7gDnfHGzS33pfQbxMO
ThiwdAX4SpN6rs2dZGpI/qNwA0sS2NhXGNm7ozUwMzAOBgNVHQ8BAf8EBAMCBaAw
EwYDVR0lBAwwCgYIKwYBBQUHAwIwDAYDVR0TAQH/BAIwADAKBggqhkjOPQQDAwNn
ADBkAjB4QSY3y5U08XfwWR3eFZU2fH4IDVf3GHEBIaEtNlXzBqBCyrOcUz343hI4
Nq1cnl8CMBhlQlZZuq8pgMkj3kCwBREpfCFxfOu0b1PMOW/4mX9NSvh44L4u3Qpn
QnQF/nCEsw==
-----END CERTIFICATE-----
EOT

  depends_on = []
}

import {
  to = incus_certificate.oc-trusted-b4e08ef4c7fb
  id = "b4e08ef4c7fb1c14b4b73422e9375fa3bdd992836ed58bc78673dcd532083ad0"
}

resource "null_resource" "post_certificates" {
  depends_on = [
    incus_certificate.oc-trusted-b4e08ef4c7fb,
  ]
}
