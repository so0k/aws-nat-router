package router

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/so0k/aws-nat-router/pkg/discover"
)

// Router interface to manage NAT Instances and VPC Routes
type Router interface {
	// UpsertNatRoute replace or create a route through specified Instance Id
	// return nil if successful or the AWS error
	UpsertNatRoute(destinationCidrBlock, instanceId, routeTableId string) error
	// PreventSourceDestCheck checks if source_dest_check is enabled on the instance interface and disables it
	PreventSourceDestCheck(instanceId string) error
}

// NatInstanceAllocation holds a list of all the routingTables allocated to a specific NatInstance
type NatInstanceAllocation struct {
	NatInstance   discover.NatInstance
	RoutingTables []discover.RoutingTable
}

// AllocateRoutes allocates RoutingTables to available NatInstances
// this function assumes the passed in list of NatInstances are all healthy
func AllocateRoutes(nis []discover.NatInstance, rts []discover.RoutingTable) []*NatInstanceAllocation {
	if len(nis) < 1 {
		return nil
	}

	var all []*NatInstanceAllocation
	// index routes by zone
	zoned := make(map[string][]*NatInstanceAllocation)
	for _, ni := range nis {
		r := &NatInstanceAllocation{
			NatInstance: ni,
		}
		all = append(all, r)
		zoned[ni.Zone] = append(zoned[ni.Zone], r)
	}

	for _, rt := range rts {
		if zni, ok := zoned[rt.Zone]; ok {
			allocateRouteToLeast(rt, zni) // allocate rt to NatInstance in same zone
		} else {
			allocateRouteToLeast(rt, all) // no NatInstance in zone, allocate rt to any zone
		}
	}
	return all
}

// allocateRouteToLeast will find the NatInstance with least allocated routing tables to allocate to
func allocateRouteToLeast(rt discover.RoutingTable, nrs []*NatInstanceAllocation) {
	// find NatRoute with least routing tables
	c := nrs[0]
	for _, i := range nrs {
		if len(i.RoutingTables) < len(c.RoutingTables) {
			c = i
		}
	}
	// append routing table for this NatInstance
	c.RoutingTables = append(c.RoutingTables, rt)
}

// AwsRouter implements Router interface for AWS
type AwsRouter struct {
	ec2 ec2iface.EC2API
}

// NewAwsRouter returns AwsRouter for ec2 svc
func NewAwsRouter(svc ec2iface.EC2API) (*AwsRouter, error) {
	return &AwsRouter{
		ec2: svc,
	}, nil
}

// UpsertNatRoute replace or create a route through specified Instance Id
func (r *AwsRouter) UpsertNatRoute(destinationCidrBlock string, ni discover.NatInstance, rt discover.RoutingTable) error {
	input := &ec2.ReplaceRouteInput{
		DestinationCidrBlock: aws.String(destinationCidrBlock),
		InstanceId:           aws.String(ni.Id),
		RouteTableId:         aws.String(rt.Id),
	}

	log.Debugf("Routing %v (%v) via %v (%v)", rt.Id, rt.Zone, ni.Id, ni.Zone)
	_, err := r.ec2.ReplaceRoute(input)
	if err != nil {
		// if replace route failed, maybe the route didn't exist
		input := &ec2.CreateRouteInput{
			DestinationCidrBlock: aws.String(destinationCidrBlock),
			InstanceId:           aws.String(ni.Id),
			RouteTableId:         aws.String(rt.Id),
		}

		_, err := r.ec2.CreateRoute(input)
		if err != nil {
			return errors.Wrap(err, "Unable to udpate route")
			// if aerr, ok := err.(awserr.Error); ok {
			// 		switch aerr.Code() {
			// 		default:
			// 				fmt.Println(aerr.Error())
			// 		}
			// } else {
			// 		// Print the error, cast err to awserr.Error to get the Code and
			// 		// Message from an error.
			// 		fmt.Println(err.Error())
			// }
		}
		log.Debugf("\tCreated")
		return nil
	}
	log.Debugf("\tUpdated")
	return nil
}
