package discover

import (
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
)

const clusterTag = "aws-nat-router/id"
const zoneTag = "aws-nat-router/zone"

// Finder interface to find cloud resources
type Finder interface {
	// FindNatInstances returns a list of Nat Instances tagged for router
	FindNatInstances(clusterId, vpcId string) ([]NatInstance, error)
	// FindRoutingTables returns a list of Routing Tables tagged for router
	FindRoutingTables(clusterId, vpcId string) ([]RoutingTable, error)
}

// AwsFinder implements Finder interface for AWS
type AwsFinder struct {
	ec2 ec2iface.EC2API
}

// NewAwsFinder returns Awsfinder for ec2 svc
func NewAwsFinder(svc ec2iface.EC2API) (*AwsFinder, error) {
	return &AwsFinder{
		ec2: svc,
	}, nil
}
