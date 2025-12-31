resource "google_dns_record_set" "github_pages_cname" {
  name         = "helm.tensorleap.ai."
  project      = "tensorleap-admin-3"
  type         = "CNAME"
  ttl          = 300
  managed_zone = "tensorleap-ai"
  rrdatas      = ["tensorleap.github.io."]
}
