output "bucket_name" {
  value = google_storage_bucket.snapshots.name
}

output "bucket_url" {
  value = google_storage_bucket.snapshots.url
}
