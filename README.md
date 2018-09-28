# AWS NAT Route Controller

Note: This is alpha software

## Overview

This controller will discover tagged NAT Instances and Routing Tables, filter down to healthy NAT Instances and allocate egress routes through the available NAT Instances for each Routing Table. To ensure only 1 router updates the routes, instances are sorted by LaunchDate and the oldest healthy instance will be considered the leader.

Following tags are expected on both EC2 Instance and Routing Table resources:

| Key                  | Description                                      | Default |
|----------------------|--------------------------------------------------|---------|
|`aws-nat-router/id`   | Multiple controller can watch multiple resources | `squid` |
|`aws-nat-router/zone` | Used to simplify zone lookup of Instance / rtb   | `-`     |

## Allocation algorithm

Currently, the router will prefer to allocate the NAT Instance in the same zone as the routing table.
If there is no healthy NAT Instance in the same zone, it will allocate to any NAT Instance which has the least routing tables.
If there are multiple healthy NAT Instances per zone, it will try to allocate the routing tables equally across all available NAT Instances

# Terraform Instance Profile

`aws-nat-router` should run on each NAT Instance, which requires the following rights:

```hcl
actions = [
      "ec2:DescribeInstances",
      "ec2:DescribeRouteTables",
      "ec2:CreateRoute",
      "ec2:ReplaceRoute",
      "ec2:ModifyInstanceAttribute", # to disable SourceDestChecks on Instances launched through ASGs
    ]
```

A more complete Instance Role setup would look like this:

```hcl
resource "aws_iam_instance_profile" "router" {
  name = "nat-router-role"
  role = "${aws_iam_role.router.name}"
}

resource "aws_iam_role" "router" {
  name               = "nat-router-role"
  assume_role_policy = "${data.aws_iam_policy_document.assume_ec2_role.json}"
}

data "aws_iam_policy_document" "assume_ec2_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "ec2_router" {
  name   = "nat-router-role-ec2"
  role   = "${aws_iam_role.squid.name}"
  policy = "${data.aws_iam_policy_document.ec2_router.json}"
}

data "aws_iam_policy_document" "ec2_router" {
  statement {
    sid = "1"

    actions = [
      "ec2:DescribeInstances",
      "ec2:DescribeRouteTables",
      "ec2:CreateRoute",
      "ec2:ReplaceRoute",
      "ec2:ModifyInstanceAttribute",
    ]

    resources = [
      "*",
    ]
  }
}
```

## Todo

`runOnce` implementation:

- [x] Discover Tagged Instances
- [x] Discover Tagged Routing Tables
- [x] Implement TCP HealthCheck
- [x] Filter down to only Healthy NAT Instances
- [x] Implement `PreventSourceDestCheck`
- [x] Allocate Routing Tables to Instances
- [x] Update Routing Tables with allocations
- [ ] Implement recovery actions (Restart or Terminate unhealthy nodes)

Controller implementation:

- [x] Use AWS secrets from commandline args / env vars or ec2 Role
- [x] Take region / vpc-id / cluster-id arguments for discovery
- [ ] Take interval arguments and loop `runOnce` on interval

## Reference

based on AWS `nat_monitor.sh`
