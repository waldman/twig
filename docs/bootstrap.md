# Bootstrap

One-time setup of the S3 state bucket that twig uses as its backend. Run
this before any `twig plan` or `twig apply`.

## Why a separate Terraform module

The state bucket can't manage its own state — classic chicken-egg. The
`bootstrap/` module sidesteps this by having no `backend` block: Terraform
stores its state locally. One resource, run once, then frozen.

## Locking

Terraform >= 1.10 uses native S3 object locking (`use_lockfile = true`) —
no DynamoDB table required. The bucket versioning enabled by the bootstrap
module is a prerequisite for this.

## Directory layout

```
bootstrap/
├── versions.tf      # provider config; no backend block
├── variables.tf     # profile, region, bucket_name
├── main.tf          # s3 bucket + versioning + encryption + public-access-block
├── outputs.tf       # bucket_name, bucket_arn
└── .gitignore       # un-ignores terraform.tfstate (safe to commit — no secrets)
```

## Files

**`versions.tf`**

```hcl
terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  profile = var.profile
  region  = var.region
}
```

**`variables.tf`**

```hcl
variable "profile" {
  type = string
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "bucket_name" {
  type     = string
  nullable = false
}
```

**`main.tf`**

```hcl
resource "aws_s3_bucket" "state" {
  bucket        = var.bucket_name
  force_destroy = false
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket = aws_s3_bucket.state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
```

**`outputs.tf`**

```hcl
output "bucket_name" {
  value = aws_s3_bucket.state.id
}

output "bucket_arn" {
  value = aws_s3_bucket.state.arn
}
```

**`bootstrap/.gitignore`**

```gitignore
.terraform/
.terraform.lock.hcl
*.tfplan

# State file committed intentionally — no secrets; records what was created
!terraform.tfstate
```

## Run it

```bash
cd bootstrap/
terraform init
terraform apply -var="profile=myprofile" -var="bucket_name=my-terraform-state"
```

The outputs give you exactly what to paste into `twig.yaml`:

```yaml
backend:
  bucket:       my-terraform-state   # ← bucket_name output
  region:       us-east-1
  profile:      myprofile
  use_lockfile: true
```

## Commit the state file

```bash
git add bootstrap/terraform.tfstate bootstrap/.gitignore
git commit -m "bootstrap: state backend created"
```

The state file contains no secrets — bucket names and ARNs are not sensitive.
Committing it records what was created and lets you verify there's no drift.

## After bootstrap

The `bootstrap/` directory is frozen. Do not run `plan` or `apply` again
unless intentionally recreating the backend from scratch.

`force_destroy = false` on the bucket means `terraform destroy` will fail
while twig state files exist in the bucket — a hard stop against accidental
teardown.
