output "queue_name" {
  value = google_cloud_tasks_queue.jobs.name
}

output "queue_id" {
  value = google_cloud_tasks_queue.jobs.id
}
