---
prev:
  text: Install
  link: /start/install
next:
  text: Your first upload
  link: /start/first-upload
---

# AWS setup & credentials

CargoShip talks to S3 in **your** AWS account using the standard AWS credential
chain — the same one the AWS CLI and SDKs use. If `aws s3 ls` works in your
shell, CargoShip will authenticate the same way.

## Credentials

CargoShip resolves credentials in the usual order:

1. Environment variables — `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
   `AWS_SESSION_TOKEN`.
2. A named profile — `AWS_PROFILE`, or `--profile` on commands that support it.
3. Shared config/credentials files — `~/.aws/credentials`, `~/.aws/config`.
4. IAM roles — EC2 instance profiles, ECS task roles, or SSO.

The quickest way to get set up locally:

```bash
aws configure
# prompts for Access Key ID, Secret Access Key, default region, output format
```

## Region

Set a default region so you don't have to pass `--region` every time:

```bash
export AWS_REGION=us-west-2
```

Most commands also accept `--region`/`-r`. If a bucket lives in a different
region than your default, pass it explicitly.

## Minimal IAM policy

To upload, inspect, and restore, the identity needs read/write on the target
bucket. A minimal policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CargoShipBucketLevel",
      "Effect": "Allow",
      "Action": ["s3:ListBucket", "s3:GetBucketLocation"],
      "Resource": "arn:aws:s3:::my-bucket"
    },
    {
      "Sid": "CargoShipObjectLevel",
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::my-bucket/*"
    }
  ]
}
```

Add these only if you use the matching features:

- **Glacier/Deep Archive restores** — `s3:RestoreObject`.
- **KMS encryption** — `kms:GenerateDataKey`, `kms:Decrypt` on your key.
- **Lifecycle policies** — `s3:PutLifecycleConfiguration`,
  `s3:GetLifecycleConfiguration`.
- **Real-time pricing / cost analysis** — `pricing:GetProducts` (and
  `ce:GetCostAndUsage` for some cost reports).
- **CloudWatch alerts** — `cloudwatch:PutMetricData`.

::: tip Least privilege
Scope `Resource` to the exact bucket and prefix you archive into rather than
`*`. You can grant `s3:DeleteObject` only if you plan to use
[`delete`/`scuttle`](/reference/commands/destructive) — omit it for
upload-only roles.
:::

## Verify access

```bash
aws s3 ls s3://my-bucket/          # can you see the bucket?
cargoship config --validate-detailed   # checks creds + bucket access
```

## Next

- [Your first upload](/start/first-upload)
- [Config files & precedence](/guides/config/files) — persist region, bucket, and defaults.
