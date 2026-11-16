# Running on EC2

Two EC2 network facts decide how fast CargoShip uploads from an instance, and
what the upload costs. Neither is visible from inside the instance. Both come
from the sister project aws-elastic-gcs's measurements (November 2026, us-west-2).

## 1. Send S3 traffic through a gateway endpoint

From inside a VPC, S3 traffic leaves one of three ways:

| Path | Throughput limit | Extra cost |
|---|---|---|
| **S3 gateway VPC endpoint** | The instance's full network bandwidth | None |
| Internet gateway (public subnet) | **5 Gbps** for instances under 32 vCPUs (except the `*8in` families) | None for S3 in-region |
| NAT gateway (private subnet) | The NAT gateway's own bandwidth | **A per-GB processing charge** on every byte uploaded (about $0.045/GB in us-east-1 at the time of writing) |

Without a gateway endpoint, a 12.5 Gbps instance can be held to 5 Gbps, and a
private subnet pays NAT processing on the whole archive. A gateway endpoint is
free and fixes both.

Check whether your instance's route table has one:

```bash
aws ec2 describe-vpc-endpoints \
  --filters Name=vpc-id,Values=<vpc-id> Name=service-name,Values=com.amazonaws.<region>.s3 \
  --query 'VpcEndpoints[].{id:VpcEndpointId,type:VpcEndpointType,routes:RouteTableIds}'
```

An entry of type `Gateway` whose `routes` include your subnet's route table means
S3 traffic bypasses the internet and NAT gateways.

## 2. Pick an instance whose network keeps up

- **The `*8in` families** (`c8in`, `m8in`, `r8in` and their `e`/`d` variants) are
  the only types under 32 vCPUs that get their full network baseline to the
  internet. A `c8in.2xlarge` (8 vCPU) has a 12.5 Gbps baseline.
- **Most other types under 32 vCPUs** cap internet-bound traffic at 5 Gbps. That
  matters only if S3 traffic isn't going through a gateway endpoint (above).
- **Low-baseline and burstable sizes** sustain far less than their "up to"
  figure once burst credits run out. Size for the baseline.
- **AMD** types used about 40% less CPU per Gbps in those measurements, but are
  capped at 5 Gbps or below at these sizes. They make sense when CPU, not the
  network, is the limit.

CargoShip compresses on the instance, so compression level and CPU count matter
too. See [Performance tuning](/guides/features/optimization).

## 3. Data transfer dominates cost

Uploading into S3 is free, and `cargoship estimate` assumes that path. It
**doesn't** include NAT gateway processing, or charges for data that crosses
regions or leaves AWS. In aws-elastic-gcs's campaign, $77 of $103 was data
transfer. If your instance and bucket are in different regions, or traffic goes
through a NAT gateway, add those charges to the estimate yourself.

## Checklist

1. An S3 gateway endpoint on the instance's route table, so neither the internet
   gateway nor a NAT gateway is in the path.
2. The bucket in the same region as the instance.
3. An instance whose **baseline** network fits the target rate: a `*8in` type
   if you need more than 5 Gbps below 32 vCPUs.
4. `--congestion-control` matters little inside a region. It's for long or lossy
   paths, such as a NAS uploading over a WAN.
