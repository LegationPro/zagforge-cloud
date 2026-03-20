output "load_balancer_ip" {
  description = "Load balancer static IP address"
  value       = local.lb_enabled ? google_compute_global_address.api[0].address : ""
}
