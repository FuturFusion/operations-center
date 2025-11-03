terraform {
  required_version = ">=1.5.7"

  required_providers {
    incus = {
      source  = "lxc/incus"
      version = "~>1.0.0"
    }

    random = {
      source  = "hashicorp/random"
      version = "~>3.7.2"
    }

    null = {
      source  = "hashicorp/null"
      version = "~>3.2.4"
    }
  }
}

provider "incus" {
  // Automatically generate the Incus client certificate if it does not exist.
  // This can also be set with the INCUS_GENERATE_CLIENT_CERTS Environment variable.
  //generate_client_certificates = true

  // Automatically accept the Incus remote's certificate.
  // If this is not set to true, you must accept the certificate out of band of Terraform.
  // This can also be set with the INCUS_ACCEPT_SERVER_CERTIFICATE environment variable.
  accept_remote_certificate = true

  default_remote = "foobar"

  remote {
    name    = "foobar"
    address = "https://127.0.0.1:8443"
  }
}

provider "random" {}

provider "null" {}
