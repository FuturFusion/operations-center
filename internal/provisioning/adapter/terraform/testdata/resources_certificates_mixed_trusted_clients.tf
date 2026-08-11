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

# The cluster does already trust the following client certificates, e.g. because
# they have been applied with the seed config of the servers. They are therefore
# imported into the Terraform state instead of being added to the cluster.

resource "incus_certificate" "oc-trusted-11f365352f3d" {
  name        = "oc-trusted-11f365352f3d"
  description = "Client trusted by Operations Center"
  restricted  = false
  type        = "client"

  projects = []

  certificate = <<EOT
-----BEGIN CERTIFICATE-----
MIIB5zCCAWygAwIBAgIUTrUg32+ydT9x27MVgT61VvsacBcwCgYIKoZIzj0EAwMw
OTEZMBcGA1UECgwQTGludXggQ29udGFpbmVyczEcMBoGA1UEAwwTb3RoZXJAaG9z
dC5zb21lLnRsZDAeFw0yNjA4MTEwODU5MjlaFw0zNjA4MDgwODU5MjlaMDkxGTAX
BgNVBAoMEExpbnV4IENvbnRhaW5lcnMxHDAaBgNVBAMME290aGVyQGhvc3Quc29t
ZS50bGQwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAARtugZrtkt6eRSpoYTn26i+ufkj
j85qBSCIE/EKFYpcFasWrdL6+qzUqKW38POXfZ8FFamhjEl3t+k4fxFd/GV2Atzi
HiYGT5YeevJ4WHb3c9XNJ9G2EYCVlDGyJ1SHkFqjNTAzMA4GA1UdDwEB/wQEAwIF
oDATBgNVHSUEDDAKBggrBgEFBQcDAjAMBgNVHRMBAf8EAjAAMAoGCCqGSM49BAMD
A2kAMGYCMQDYuISb/SbY78RKEHecDDx67BAEW/C0OxmSiQJU9a1FRNqhUwRTI9Sx
d6vQ1sIxvb8CMQCI5TiIxdWQn/AHlguC94RrOBbNAfQ2oGm7+ci6KnX8mGEwZ4vW
4KMOxXCIti9xX6w=
-----END CERTIFICATE-----
EOT

  depends_on = []
}

import {
  to = incus_certificate.oc-trusted-11f365352f3d
  id = "11f365352f3d4e2c4d501b6b5786457627dbd525689aa86a2623069a4d840765"
}

resource "null_resource" "post_certificates" {
  depends_on = [
    incus_certificate.oc-trusted-b4e08ef4c7fb,
    incus_certificate.oc-trusted-11f365352f3d,
  ]
}
