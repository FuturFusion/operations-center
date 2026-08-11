# The cluster does already trust the following client certificates, e.g. because
# they have been applied with the seed config of the servers. They are therefore
# not managed by this Terraform configuration.
#
# To take such a certificate under management, import it into the Terraform state
# and uncomment the respective resource.

# resource "incus_certificate" "oc-trusted-b4e08ef4c7fb" {
#   name        = "oc-trusted-b4e08ef4c7fb"
#   description = "Client trusted by Operations Center"
#   restricted  = false
#   type        = "client"
#
#   projects = []
#
#   certificate = <<EOT
# -----BEGIN CERTIFICATE-----
# MIIB4zCCAWqgAwIBAgIUZT1U82k44TuXkKcq1jdXYxaZDDgwCgYIKoZIzj0EAwMw
# ODEZMBcGA1UECgwQTGludXggQ29udGFpbmVyczEbMBkGA1UEAwwSdXNlckBob3N0
# LnNvbWUudGxkMB4XDTI2MDgxMTA2MTgyOFoXDTM2MDgwODA2MTgyOFowODEZMBcG
# A1UECgwQTGludXggQ29udGFpbmVyczEbMBkGA1UEAwwSdXNlckBob3N0LnNvbWUu
# dGxkMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEW9cy2pfFn/i3XdgNGl5WqLkthPHS
# 4x5zQubllNA66BAcV23vVqJ0xXMfKctlVgrU0NRSNEn5mX7gDnfHGzS33pfQbxMO
# ThiwdAX4SpN6rs2dZGpI/qNwA0sS2NhXGNm7ozUwMzAOBgNVHQ8BAf8EBAMCBaAw
# EwYDVR0lBAwwCgYIKwYBBQUHAwIwDAYDVR0TAQH/BAIwADAKBggqhkjOPQQDAwNn
# ADBkAjB4QSY3y5U08XfwWR3eFZU2fH4IDVf3GHEBIaEtNlXzBqBCyrOcUz343hI4
# Nq1cnl8CMBhlQlZZuq8pgMkj3kCwBREpfCFxfOu0b1PMOW/4mX9NSvh44L4u3Qpn
# QnQF/nCEsw==
# -----END CERTIFICATE-----
# EOT
#
#   depends_on = []
# }
