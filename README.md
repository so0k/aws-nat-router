# AWS NAT Route Controller

Note: This is alpha software

## Overview

This controller will discover tagged NAT Instances and Routing Tables, filter down to healthy NAT Instances and allocate egress routes through the available NAT Instances for each Routing Table.

Following tags are expected on both EC2 Instance and Routing Table resources:

| Key                  | Description                                      | Default |
|----------------------|--------------------------------------------------|---------|
|`aws-nat-router/id`   | Multiple controller can watch multiple resources | `squid` |
|`aws-nat-router/zone` | Used to simplify zone lookup of Instance / rtb   | `-`     |

## Allocation algorithm

Currently, the router will prefer to allocate the NAT Instance in the same zone as the routing table.
If there is no healthy NAT Instance in the same zone, it will allocate to any NAT Instance which has the least routing tables.
If there are multiple healthy NAT Instances per zone, it will try to allocate the routing tables equally across all available NAT Instances

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
