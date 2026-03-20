terraform {
  backend "gcs" {
    bucket = "zagforge-terraform-state"
  }
}
