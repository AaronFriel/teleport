---
authors: Logan Davis (logan.davis@goteleport.com)
state: draft
---

# RFD 73 - Public Image Registry

## What
Teleport images are currently hosted on [Quay.io](https://quay.io/organization/gravitational). This RFD proposes migrating public images from Quay to [Amazon ECR](https://aws.amazon.com/ecr/). 

## Why
As of August 1st, 2021 Quay.io no longer supports any other authentication provider other than Red Hat Single-Sign On. Users in the Gravitational organization on Quay must be manually removed when they leave Teleport which presents a potential security risk. Migrating to Amazon ECR will consolidate our infrastructure while also improving security with support for IAM policies and our existing SSO infrastructure.

## Details
Teleport will use Amazon ECR and Amazon ECR Public in order to host public container images. The multiple stage registry pipeline will allow Teleport to test and verify images internally before promoting them to our customers. As of authoring this RFD, Amazon ECR Public lacks support for [vulnerability scanning and tag immutabilty](https://github.com/aws/containers-roadmap/issues/1288). The two-stage pipeline will allow us to leverage these features in the internal repository before pushing to the public. 

**What about name squatting on other container registry platforms?**

A not yet finished RFD on third-party artifact mirroring will address concerns regarding name squatting on other container registry platforms. See [Artifact Mirroring](https://github.com/gravitational/teleport/commit/2262efbb25463ccc135553d43293f6d8aee22ba2).

### Scope

This RFD will focus on the infrastructure, security, and observability of the replacement registry. It will also include a deprecation and migration plan for the existing Quay.io repositories.

#### In Scope
* Infrastructure plans with [example terraform](#appendix-a-example-terraform)
* Migration and Deprecation plan

#### Out of Scope
* Image Signing w/ Cosign
* Mirroring of images to third-party registries. See [Artifact Mirroring](https://github.com/gravitational/teleport/commit/2262efbb25463ccc135553d43293f6d8aee22ba2).

### Infrastructure
```
             ┌─────────────────────────────────────────────────────────┐
             │                                                         │
             │    ┌───────────┐                   ┌──────────────────┐ │
   Tag / Push│    │           │     Promotion     │                  │ │
─────────────┼─►  │  AWS ECR  │  ─────────┬────►  │  AWS ECR Public  │ │◄───────── public.ecr.aws/gravitational/teleport
             │    │           │           │       │                  │ │
             │    └───────────┘           │       └──────────────────┘ │
             │                            │                            │
             │AWS Account: teleport-prod  │                            │
             └────────────────────────────┼────────────────────────────┘
                                          │
                                          │                ┌───────────┐
                                          │                │           │
                                          └──────────────► │  Quay.io  │◄───────── quay.io/gravitational/teleport
                                                           │           │
                                                           └───────────┘
```

The infrastructure for this will live in the [cloud-terraform](https://github.com/gravitational/cloud-terraform) repository. The terraform for the `teleport-prod` account can be found [here](https://github.com/gravitational/cloud-terraform/tree/main/teleport-team/prod). Using AWS ECR and ECR Public allow us to rely on their managed infrastructure which reduces the operational complexity while enforcing our own security policies and allowing us to better audit changes to the environment. For more information on the pros and cons of alternatives, see [alternatives](#alternatives).

### Security
Amazon ECR and ECR Public have support for AWS IAM. With IAM we can create least-privileged policies that allow limited access to one or more part of the container-registry. For an example promotion user policy, see the [terraform example](#appendix-a-example-terraform). 

In addition to AWS IAM, AWS supports our existing SSO infrastructure with Okta.

As a part of observability, Cloudtrail logs will log all interactions with ECR which will allow the security team to create alerts for any changes to images. 

### Observabilty
Amazon ECR provides detailed usage metrics through [Cloudwatch](https://docs.aws.amazon.com/AmazonECR/latest/userguide/monitoring-usage.html) as well as detailed logging through AWS [Cloudtrail](https://docs.aws.amazon.com/AmazonECR/latest/userguide/logging-using-cloudtrail.html). 

For Amazon ECR Public, observability is limited. Currently, you can log authenticated requests via [Cloudtrail](https://docs.aws.amazon.com/AmazonECR/latest/public/logging-using-cloudtrail.html). An open issue exists for better metrics for ECR Public, see [this](https://github.com/aws/containers-roadmap/issues/1587). 

### Migration and Deprecation
* Using the list of public teleport images defined [below](#appendix-b-teleport-public-images), the terraform infrastructure needed for these registries will be created according to the standards defined in[artifact storage standards](https://github.com/gravitational/cloud/blob/9124947fdfb0773fa9bd567160481bed4ec84b7e/rfd/0017-artifact-storage-standards.md#levels).
* Existing CI/CD pipelines and tooling will be updated to push to both Quay.io and ECR. 
* Documentation will be updated to reflect the new registry location. 
* Quay.io images will be deprecated and removed in reverse level order. (Bronze -> Silver -> Gold)

Gold standard images (teleport, teleport-ent, etc...) will continue to exist and be pushed to Quay.io for the foreseeable future. [Artifact Mirroring](https://github.com/gravitational/teleport/commit/2262efbb25463ccc135553d43293f6d8aee22ba2) will go into more details. 

### Alternatives
#### Self hosted with Docker Distribution or Harbor
The [Docker Distribution](https://github.com/distribution/distribution) registry is an open source implementation of the [OCI Distribution](https://github.com/opencontainers/distribution-spec) specification. [Harbor](https://goharbor.io/) is an OSS, _all-in-one_, registry solution that is built on top of the docker registry. Harbor has a built-in UI, support for OIDC authenticatio and many more [additional features](https://goharbor.io/docs/2.5.0/). 

While Harbor provides a maximally featured container registry solution, it also incurs an increased operational overhead that Teleport didn't have with Quay.io.

#### Custom Registry w/ CloudFront Functions
A minimal, _oci-compatible_ registry could be implemented through just CloudFront functions. This registry would only support reading. This would reduce the operational complexity of the current strategy to AWS specific components. Additional components would be needed to be developed in order to push the image to the S3 bucket but could be implemented as just another step in the existing CI/CD pipeline.

A negative to this solution is the lack of features that come standard with other registry solutions. This includes, but is not limited to, vulnerability scanning and tag immutability. Additionally, discoverability would be a missing feature from this solution.

### Appendix A: Example Terraform
```hcl
# Internal Repository for Teleport
resource "aws_ecr_repository" "teleport" {
  name                 = "gravitational/teleport"
  image_tag_mutability = "IMMUTABLE"

  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.ecr_key.arn
  }

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Public Repository for Teleport
resource "aws_ecrpublic_repository" "teleport" {
  repository_name = "teleport"

   catalog_data {
      ...
   }
}

# Promotion User Policy
data "aws_iam_policy_document" "teleport_promotion_user" {
  statement {
    sid    = "AllowPushImages"
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:CompleteLayerUpload",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart"
    ]
    resources = [aws_ecrpublic_repository.teleport.arn]
  }
  statement {
    sid    = "AllowPullImages"
    effect = "Allow"
    actions = [
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
    ]
    resources = [aws_ecr_repository.teleport.arn]
  }
  statement {
    sid    = "AllowAuthToken"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "teleport_promotion_user" {
  name        = "teleport_promotion_user"
  path        = "/"
  description = "Amazon ECR Policy for promoting teleport from internal ECR to public ECR."
  policy      = data.aws_iam_policy_document.teleport_promotion_user
}

resource "aws_iam_user" "teleport_promotion_user" {
  name = "teleport_promotion_user"
}

resource "aws_iam_user_policy_attachment" "teleport_promotion_user" {
  user       = aws_iam_user.teleport_promotion_user.name
  policy_arn = aws_iam_policy.teleport_promotion_user.arn
}
```

### Appendix B: Teleport Public Images
The following table represents a best guess guide to migration of existing images from Quay to Harbor. They have been marked as such given their perceived relevance based on Quay activity and number of references in the Gravitational organization. 

#### **Repositories to migrate and mirror**
| Repositories |
| ---- |
| teleport |
| teleport-ent |
| teleport-plugin-email |
| teleport-plugin-event-handler |
| teleport-plugin-jira |
| teleport-plugin-mattermost |
| teleport-plugin-pagerduty |
| teleport-plugin-slack |

#### **Repositories to migrate, but not mirror**
| Repositories |
| ---- |
| aws-ecr-helper |
| buildbox-base |
| fpm-centos |
| fpm-debian |
| next |
| prometheus-operator |
| teleport-buildbox |
| teleport-buildbox-arm |
| teleport-buildbox-arm-fips | 
| teleport-buildbox-fips |
| teleport-ent-dev |
| teleport-lab |

### **Repositories to not migrate**

| Repositories |
| ---- |
| debian-grande |
| debian-tall |
| debian-venti |
| kube-router | 
| gravity-scan |
| pithos |
| pithosctl |
| pithos-proxy |
| planet |
| robotest |
| robotest-e2e |
| robotest-suite |
| rig |
| satellite |
| stolon | 
| stolonctl |
| stolon-etcd |
| stolon-pgbouncer | 
| ubuntu-grande | 
| ubuntu-tall | 
| ubuntu-venti |
| wormhole | 
| wormhole-dev |

### **Repositories to delete**
| Repositories |
| ---- |
| alpine |
| alpine-glibc |
| busyloop |
| cve-2018-1002105 |
| docker-alpine-build |
| docker-gc |
| drone-fork-approval-extension | 
| force |
| kaniko-init-container |
| keygen |
| mattermost-worker |
| mkdocs-base |
| monitoring-grafana | 
| monitoring-influxdb |
| netbox |
| nethealth-dev | 
| nginx | 
| nginx-controller |
| provisioner |
| s3-mounter |
| slackbot |
| stress |
| sync-kubeconfig |
| sync-kubesecrets |
| teleport-buildbox-centos6 |
| teleport-buildbox-centos6-fips |
| teleport-dev | 
| tube |
| watcher | 
