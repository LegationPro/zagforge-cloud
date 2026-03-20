# Networking module — Load Balancer + Cloud Armor
# Only created when a custom domain is provided.

locals {
  lb_enabled = var.domain != ""
}

# --- Static IP ---
resource "google_compute_global_address" "api" {
  count = local.lb_enabled ? 1 : 0
  name  = "${var.name_prefix}-api-ip"
}

# --- Managed SSL Certificate ---
resource "google_compute_managed_ssl_certificate" "api" {
  count = local.lb_enabled ? 1 : 0
  name  = "${var.name_prefix}-api-cert"

  managed {
    domains = [var.domain]
  }
}

# --- Serverless NEG (Cloud Run backend) ---
resource "google_compute_region_network_endpoint_group" "api" {
  count                 = local.lb_enabled ? 1 : 0
  name                  = "${var.name_prefix}-api-neg"
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = var.api_cloud_run_name
  }
}

# --- Backend Service ---
resource "google_compute_backend_service" "api" {
  count       = local.lb_enabled ? 1 : 0
  name        = "${var.name_prefix}-api-backend"
  protocol    = "HTTPS"
  timeout_sec = 30

  backend {
    group = google_compute_region_network_endpoint_group.api[0].id
  }

  security_policy = var.cloud_armor_enabled ? google_compute_security_policy.api[0].id : null
}

# --- URL Map ---
resource "google_compute_url_map" "api" {
  count           = local.lb_enabled ? 1 : 0
  name            = "${var.name_prefix}-api-urlmap"
  default_service = google_compute_backend_service.api[0].id
}

# --- HTTPS Proxy ---
resource "google_compute_target_https_proxy" "api" {
  count            = local.lb_enabled ? 1 : 0
  name             = "${var.name_prefix}-api-https-proxy"
  url_map          = google_compute_url_map.api[0].id
  ssl_certificates = [google_compute_managed_ssl_certificate.api[0].id]
}

# --- Forwarding Rule ---
resource "google_compute_global_forwarding_rule" "api" {
  count      = local.lb_enabled ? 1 : 0
  name       = "${var.name_prefix}-api-forwarding-rule"
  target     = google_compute_target_https_proxy.api[0].id
  port_range = "443"
  ip_address = google_compute_global_address.api[0].address
}

# --- HTTP → HTTPS redirect ---
resource "google_compute_url_map" "http_redirect" {
  count = local.lb_enabled ? 1 : 0
  name  = "${var.name_prefix}-http-redirect"

  default_url_redirect {
    https_redirect = true
    strip_query    = false
  }
}

resource "google_compute_target_http_proxy" "http_redirect" {
  count   = local.lb_enabled ? 1 : 0
  name    = "${var.name_prefix}-http-redirect-proxy"
  url_map = google_compute_url_map.http_redirect[0].id
}

resource "google_compute_global_forwarding_rule" "http_redirect" {
  count      = local.lb_enabled ? 1 : 0
  name       = "${var.name_prefix}-http-redirect-rule"
  target     = google_compute_target_http_proxy.http_redirect[0].id
  port_range = "80"
  ip_address = google_compute_global_address.api[0].address
}

# --- Cloud Armor ---
resource "google_compute_security_policy" "api" {
  count = var.cloud_armor_enabled ? 1 : 0
  name  = "${var.name_prefix}-api-policy"

  # Default rate limit: 1000 req/IP/min
  rule {
    action   = "rate_based_ban"
    priority = 1000

    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }

    rate_limit_options {
      conform_action = "allow"
      exceed_action  = "deny(429)"
      rate_limit_threshold {
        count        = 1000
        interval_sec = 60
      }
    }
  }

  # OWASP CRS — block SQLi and XSS
  rule {
    action   = "deny(403)"
    priority = 2000

    match {
      expr {
        expression = "evaluatePreconfiguredWaf('sqli-v33-stable') || evaluatePreconfiguredWaf('xss-v33-stable')"
      }
    }
  }

  # Default allow
  rule {
    action   = "allow"
    priority = 2147483647

    match {
      versioned_expr = "SRC_IPS_V1"
      config {
        src_ip_ranges = ["*"]
      }
    }
  }
}
