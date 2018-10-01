package discover

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
)

// RoutingTable holds information about a Routing Table
type RoutingTable struct {
	Id                  string
	Zone                string
	EgressNatInstanceId string
}

// FindRoutingTables returns a list of Routing Tables to route through cluster
func (r *AwsFinder) FindRoutingTables(clusterId, vpcId string) ([]*RoutingTable, error) {
	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String(fmt.Sprintf("tag:%v", clusterTag)),
				Values: []*string{
					aws.String(clusterId),
				},
			},
			{
				Name: aws.String("vpc-id"),
				Values: []*string{
					aws.String(vpcId),
				},
			},
		},
	}

	log.Debugf("Finding RoutingTables with 'tag:%v=%v' and 'vpc-id=%v'", clusterTag, clusterId, vpcId)
	result, err := r.ec2.DescribeRouteTables(input)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to find RoutingTables")
	}

	var routingTables []*RoutingTable
	for _, r := range result.RouteTables {
		rt := &RoutingTable{
			Id: *r.RouteTableId,
		}
		for _, route := range r.Routes {
			// hardcoding egress = 0.0.0.0/0
			if *route.DestinationCidrBlock == "0.0.0.0/0" && route.InstanceId != nil {
				rt.EgressNatInstanceId = *route.InstanceId
			}
		}

		for _, t := range r.Tags {
			if *t.Key == zoneTag {
				rt.Zone = *t.Value
			}
		}
		routingTables = append(routingTables, rt)
	}
	return routingTables, nil
}
