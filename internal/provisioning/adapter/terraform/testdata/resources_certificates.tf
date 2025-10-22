resource "incus_certificate" "cert1" {
  name        = "cert1"
  description = "metrics certificate 1"
  restricted  = true
  type        = "metrics"

  projects = [
    "project1",
    "project2"
  ]

  certificate = <<EOT
-----BEGIN CERTIFICATE-----
MIIB1jCCAVygAwIBAgIQaKBbJqVWID8NqSoMxF/nHzAKBggqhkjOPQQDAzAzMRkw
FwYDVQQKExBMaW51eCBDb250YWluZXJzMRYwFAYDVQQDDA1sdWJyQHN1cnZpc3Rh
MB4XDTI1MDIxMjE0MTEzMVoXDTM1MDIxMDE0MTEzMVowMzEZMBcGA1UEChMQTGlu
dXggQ29udGFpbmVyczEWMBQGA1UEAwwNbHVickBzdXJ2aXN0YTB2MBAGByqGSM49
AgEGBSuBBAAiA2IABDXH+i9i6WilQA56Qe4wvTGZL1NGDeGZFCCskJduZietB0bX
K30ug6JdxUHGfhg3CL92lTnmtMwJC+Ev+IQFhLGv/Yk/OLP4BB1zdqBgmyA7Mmwq
jcrp8B8FTBZ9AQmCe6M1MDMwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsG
AQUFBwMCMAwGA1UdEwEB/wQCMAAwCgYIKoZIzj0EAwMDaAAwZQIxAPmS67jexjgT
6PrxAo/fQpK71BwgpsHOCZHM2b3t4lZlDirjN40xNGPeNH+KG95R3wIwexlentZZ
0x2N/SJBYGltBnBjH8mm8OTWa1N/MpOAl2K7jRVuSeuWGBDf0/n+M6br
-----END CERTIFICATE-----
EOT

  depends_on = []
}

resource "null_resource" "post_certificates" {
  depends_on = [
    incus_certificate.cert1,
  ]
}
