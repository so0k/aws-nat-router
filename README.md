# AWS NAT Route Controller

Note: This is alpha software

[![CircleCI](https://circleci.com/gh/so0k/aws-nat-router.svg?style=svg)](https://circleci.com/gh/so0k/aws-nat-router)

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

Systemd Unit

```ini
[Unit]
Description=AWS NAT Router
Documentation=https://github.com/so0k/aws-nat-router
Requires=network-online.target
After=network-online.target
[Service]
# -z: request a file modified later than the given filename modification date (mtime)
ExecStartPre=/usr/bin/curl -sLo /usr/local/bin/aws-nat-router /
  -z /usr/local/bin/aws-nat-router /
  https://github.com/so0k/aws-nat-router/releases/download/0.1.4/aws-nat-router
ExecStartPre=/usr/bin/chmod +x /usr/local/bin/aws-nat-router
Environment=LOG_LEVEL=INFO
ExecStart=/usr/local/bin/aws-nat-router \
  --vpc-id ${vpc_id} \
  --cluster-id ${cluster_id} \
  --ec2-election \
  --timeout 500ms \
  --interval 5s
Restart=always
RestartSec=10
# amount of time (seconds) systemd waits after start before marking it as failed
TimeoutStartSec=20
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
- [x] Take interval arguments and loop `runOnce` on interval

Deployment:

The controller is meant to run on EC2 Instances, prior to k8s bootstrap, thus we can't use Docker / Kubernetes as a deployment mechanism.

- [x] Add GitHub release to CircleCI
- [x] Add Sample Systemd unit file

## Reference

based on AWS `nat_monitor.sh`
