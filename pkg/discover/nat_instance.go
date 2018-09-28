package discover

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// NatInstance holds information about a Nat Instance
type NatInstance struct {
	Id              string
	State           string
	PrivateIP       string
	PublicIP        string
	Zone            string
	SourceDestCheck bool
	LaunchTime      time.Time
}

// FindNatInstances returns a list of Nat Instances tagged for router
func (r *AwsFinder) FindNatInstances(clusterId, vpcId string) ([]*NatInstance, error) {
	input := &ec2.DescribeInstancesInput{
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

	var natInstances []*NatInstance
	log.Debugf("Finding Instances with 'tag:%v=%v' and 'vpc-id=%v'", clusterTag, clusterId, vpcId)
	err := r.ec2.DescribeInstancesPages(input,
		func(page *ec2.DescribeInstancesOutput, lastPage bool) bool {
			for _, res := range page.Reservations {
				for _, i := range res.Instances {
					ni := &NatInstance{
						Id:              *i.InstanceId,
						State:           *i.State.Name,
						SourceDestCheck: *i.SourceDestCheck,
						LaunchTime:      *i.LaunchTime,
					}
					if i.PrivateIpAddress != nil {
						ni.PrivateIP = *i.PrivateIpAddress
					}
					if i.PrivateIpAddress != nil {
						ni.PublicIP = *i.PublicIpAddress
					}
					for _, t := range i.Tags {
						if *t.Key == zoneTag {
							ni.Zone = *t.Value
						}
					}
					natInstances = append(natInstances, ni)
				}
			}
			// to stop iterating, return false
			return true
		})
	if err != nil {
		return nil, errors.Wrap(err, "Unable to Find Nat Instances")
	}

	return natInstances, nil
}
