# TWIG - GENERATION SPEC

## Generated main.tf structure

### 1. terraform block

```hcl
terraform {
  required_version = ">= 1.1"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  backend "s3" {
    # all fields from twig.yaml backend block, plus:
    key = "infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>/terraform.tfstate"
  }
}
```

### 2. provider block

```hcl
provider "aws" {
  profile = "<profile>"
  region  = "<region>"
}
```

### 3. remote state blocks (one per `remote_state:` entry, in declaration order)

```hcl
data "terraform_remote_state" "<alias>" {
  backend = "s3"
  config = {
    # all backend fields from twig.yaml except dynamodb_table, plus derived key:
    bucket  = "<bucket>"
    region  = "<region>"
    key     = "infra/<cloud>/.../<component>/terraform.tfstate"
  }
}
```

### 4. module blocks

One block per module entry, in declaration order:

```hcl
module "<instance_key>" {
  source = "<absolute-path-to-module>"

  cloud       = "<cloud>"
  profile     = "<profile>"
  region      = "<region>"
  environment = "<environment>"
  class       = "<class>"
  component   = "<component>"
  module      = "<instance_key>"

  # user vars with cross-refs resolved
  <variable_name> = <value>
}
```
