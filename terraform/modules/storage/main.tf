resource "google_storage_bucket" "snapshots" {
  name     = "${var.name_prefix}-snapshots"
  location = var.region

  uniform_bucket_level_access = true

  versioning {
    enabled = false
  }

  lifecycle_rule {
    condition {
      age = 90
    }
    action {
      type = "Delete"
    }
  }
}

# Worker: write-only
resource "google_storage_bucket_iam_member" "worker_writer" {
  bucket = google_storage_bucket.snapshots.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.worker_service_account}"
}

# API: read-only
resource "google_storage_bucket_iam_member" "api_reader" {
  bucket = google_storage_bucket.snapshots.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${var.api_service_account}"
}
