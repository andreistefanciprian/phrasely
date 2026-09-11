resource "cloudflare_r2_bucket" "phrase_audio" {
  account_id    = var.cloudflare_account_id
  name          = var.r2_bucket_name
  location      = "apac"
  storage_class = "Standard"
}
