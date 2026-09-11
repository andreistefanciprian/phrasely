variable "cloudflare_account_id" {
  description = "Cloudflare account ID that owns the R2 bucket."
  type        = string

  validation {
    condition     = length(trimspace(var.cloudflare_account_id)) > 0
    error_message = "cloudflare_account_id must not be empty."
  }
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token used by Terraform to manage the R2 bucket."
  type        = string
  sensitive   = true

  validation {
    condition     = length(trimspace(var.cloudflare_api_token)) > 0
    error_message = "cloudflare_api_token must not be empty."
  }
}

variable "r2_bucket_name" {
  description = "Name of the private R2 bucket used to cache generated phrase audio."
  type        = string
  default     = "phrasely-audio"

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]*[a-z0-9]$", var.r2_bucket_name)) && length(var.r2_bucket_name) >= 3 && length(var.r2_bucket_name) <= 63
    error_message = "r2_bucket_name must be 3-63 characters of lowercase letters, numbers, or hyphens, and must start and end with a letter or number."
  }
}
