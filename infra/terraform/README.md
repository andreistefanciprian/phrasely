# Terraform infrastructure

Terraform runs through Docker Compose, so no local Terraform installation is
needed. Each subdirectory is an independent Terraform root with its own provider
lock file and state. Cloudflare infrastructure is in `cloudflare/`; future
infrastructure groups can be added as sibling directories.

## Setup

Copy the example environment file and replace its placeholders:

```sh
cd infra/terraform
cp .env.example .env
```

The Cloudflare token needs `Workers R2 Storage: Edit` permission for the target
account. It manages infrastructure only and is not an R2 S3 runtime credential.
The `.env` file, Terraform state, plans, and `.terraform/` working directories
are ignored by Git.

State is currently local at `cloudflare/terraform.tfstate`. Protect and back up
that file: losing it makes Terraform lose track of the bucket it manages. A
remote backend can be added later without changing the Docker workflow.

## Commands

Run these from `infra/terraform/`:

```sh
make init
make check
make plan
make apply
make output
```

`make plan` saves the reviewed plan as `cloudflare/cloudflare.tfplan`, and
`make apply` applies that exact saved plan. Terraform does not ask for a second
approval when applying a saved plan, so review the complete `make plan` output
before running `make apply`. Plan files are ignored by Git. Provider versions
remain pinned by the committed `cloudflare/.terraform.lock.hcl`.

The default target is `cloudflare`. A future sibling configuration can be
selected explicitly:

```sh
make plan TF_TARGET=another-resource-group
```

## Private R2 audio cache

The Cloudflare configuration creates a private `phrasely-audio` bucket using
Standard storage and Cloudflare's APAC location hint. It creates no public
`r2.dev` URL, custom domain, CORS policy, or lifecycle policy. Do not enable
public access in the Cloudflare dashboard.

After applying, create a separate R2 API token in Cloudflare with Object Read &
Write access scoped only to this bucket. Store its credentials and the Terraform
outputs on the Railway service that implements Listen:

| Railway variable | Value |
| --- | --- |
| `R2_ENDPOINT` | The `r2_s3_endpoint` Terraform output |
| `R2_BUCKET` | The `r2_bucket_name` Terraform output |
| `R2_ACCESS_KEY_ID` | Access Key ID from the separately created R2 API token |
| `R2_SECRET_ACCESS_KEY` | Secret Access Key from the separately created R2 API token |

Never put the runtime Access Key ID or Secret Access Key in Terraform variables,
state, outputs, or committed files.
