resource "incus_project" "internal" {
  name        = "internal"
  description = "Internal project to isolate fully managed resources."

  config = {
  }
}

resource "null_resource" "post_projects" {
  depends_on = [
    incus_project.internal,
  ]
}
