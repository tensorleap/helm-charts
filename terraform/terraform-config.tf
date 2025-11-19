terraform {
  required_version = "~> 1.2.0"
  backend "gcs" {
    bucket = "tensorleap-infra-nonprod"
    prefix = "helm-charts-repository/"
  }
}
