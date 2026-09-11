output "r2_bucket_name" {
  description = "Name of the private R2 phrase-audio bucket."
  value       = cloudflare_r2_bucket.phrase_audio.name
}

output "r2_s3_endpoint" {
  description = "Account-scoped R2 S3 API endpoint for the application runtime."
  value       = "https://${var.cloudflare_account_id}.r2.cloudflarestorage.com"
}
