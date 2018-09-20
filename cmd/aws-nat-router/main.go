package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/so0k/aws-nat-router/pkg/discover"
	"github.com/so0k/aws-nat-router/pkg/router"
	"github.com/urfave/cli"
)

var build = "0"

func main() {
	flags := []cli.Flag{
		cli.StringFlag{
			Name:   "log-level,l",
			Value:  "error",
			Usage:  "Log level (panic, fatal, error, warn, info, or debug)",
			EnvVar: "LOG_LEVEL",
		},
		cli.StringFlag{
			Name:  "aws-access-key",
			Usage: "Optional aws access key to use",
		},
		cli.StringFlag{
			Name:  "aws-secret-key",
			Usage: "Optional aws secret key to use",
		},
		cli.StringFlag{
			Name:   "region,r",
			Value:  "ap-southeast-1",
			Usage:  "AWS `REGION`",
			EnvVar: "AWS_REGION",
		},
		cli.StringFlag{
			Name:   "vpc-id",
			Usage:  "Required `ID` of the VPC the NAT Instnaces live in",
			EnvVar: "NAT_VPC_ID",
		},
		cli.StringFlag{
			Name:   "cluster-id",
			Value:  "squid",
			Usage:  "`ID` the NAT Instances are tagged with",
			EnvVar: "NAT_CLUSTER_ID",
		},
	}
	app := cli.NewApp()
	app.Name = "aws-nat-router"
	app.Usage = "Manage AWS Nat Instance and private subnet routing tables"
	app.Action = run

	app.Version = fmt.Sprintf("0.1.%s", build)
	app.Author = "so0k"

	app.Flags = flags

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type config struct {
	awsAccessKey string
	awsSecretKey string
	region       string
	vpcId        string
	clusterId    string
}

func parseConfig(c *cli.Context) (*config, error) {
	conf := &config{
		awsSecretKey: c.String("aws-secret-key"),
		awsAccessKey: c.String("aws-access-key"),
		region:       c.String("region"),
		vpcId:        c.String("vpc-id"),
		clusterId:    c.String("cluster-id"),
	}
	lStr := c.String("log-level")
	l, err := log.ParseLevel(lStr)
	if err != nil {
		return nil, err
	}
	log.SetLevel(l)

	// validate vpc-id
	if len(conf.vpcId) == 0 {
		return nil, errors.New("vpc-id can not be blank")
	}

	//TODO: validate region?

	return conf, nil
}

func run(c *cli.Context) error {
	appConf, err := parseConfig(c)
	if err != nil {
		log.Error(err)
		cli.ShowAppHelpAndExit(c, 1)
	}
	conf := initAwsConfig(appConf.awsAccessKey, appConf.awsSecretKey, appConf.region)

	svc := ec2.New(session.New(conf))
	f, _ := discover.NewAwsFinder(svc)
	nis, _ := f.FindNatInstances(appConf.clusterId, appConf.vpcId)
	for _, ni := range nis {
		fmt.Printf("Instance: %v - Zone: %v - State: %v - PrivateIP: %v\n",
			ni.Id,
			ni.Zone,
			ni.State,
			ni.PrivateIP,
		)
	}

	rts, _ := f.FindRoutingTables(appConf.clusterId, appConf.vpcId)
	for _, rt := range rts {
		fmt.Printf("Routing Table: %v - Zone: %v\n",
			rt.Id,
			rt.Zone,
		)
	}

	// TODO: healthcecks and filter out unhealthy / stopped instances

	nias := router.AllocateRoutes(nis, rts)

	// Apply Allocations
	r, _ := router.NewAwsRouter(svc)
	for _, nia := range nias {
		for _, rt := range nia.RoutingTables {
			r.UpsertNatRoute("0.0.0.0/0", nia.NatInstance, rt)
		}
	}

	// TODO: start recovery for unhealthy instances (async... on next iteration they will be re-evaluated)
	return nil
}

func initAwsConfig(accessKey, secretKey, region string) *aws.Config {
	awsConfig := aws.NewConfig()
	creds := credentials.NewChainCredentials([]credentials.Provider{
		&credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
			},
		},
		&credentials.EnvProvider{},
		&credentials.SharedCredentialsProvider{},
	})
	awsConfig.WithCredentials(creds)
	awsConfig.WithRegion(region)
	return awsConfig
}
