package main

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/so0k/aws-nat-router/pkg/discover"
	"github.com/so0k/aws-nat-router/pkg/healthcheck"
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
		cli.BoolFlag{
			Name:   "public",
			Usage:  "Use Public IPs for health checks",
			EnvVar: "NAT_HC_PUBLIC",
		},
		cli.IntFlag{
			Name:   "port,p",
			Value:  3128,
			Usage:  "`PORT` for TCP HealthChecks",
			EnvVar: "NAT_HC_PORT",
		},
		cli.DurationFlag{
			Name:   "timeout",
			Value:  50 * time.Millisecond,
			Usage:  "`DURATION` before HealthChecks time out",
			EnvVar: "NAT_HC_TIMEOUT",
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
	public       bool
	port         int
	timeout      time.Duration
}

func parseConfig(c *cli.Context) (*config, error) {
	conf := &config{
		awsSecretKey: c.String("aws-secret-key"),
		awsAccessKey: c.String("aws-access-key"),
		region:       c.String("region"),
		vpcId:        c.String("vpc-id"),
		clusterId:    c.String("cluster-id"),
		public:       c.Bool("public"),
		port:         c.Int("port"),
		timeout:      c.Duration("timeout"),
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

	// Check liveness for each instance
	// TODO(so0k): use go routines to check in parallel
	var liveNis, deadNis []*discover.NatInstance
	for _, ni := range nis {
		var addr string
		if appConf.public {
			addr = fmt.Sprintf("%v:%v", ni.PublicIP, appConf.port)
		} else {
			addr = fmt.Sprintf("%v:%v", ni.PrivateIP, appConf.port)
		}
		err := healthcheck.TCPCheck(addr, appConf.timeout)
		if err != nil {
			log.Debugf("Instance %v (%v) is dead :(", ni.Id, addr)
			deadNis = append(deadNis, ni)
		} else {
			log.Debugf("Instance %v (%v) is alive!", ni.Id, addr)
			liveNis = append(liveNis, ni)
		}
	}
	rts, _ := f.FindRoutingTables(appConf.clusterId, appConf.vpcId)

	// Allocate routes to live NATInstances
	nias := router.AllocateRoutes(liveNis, rts)

	// Apply Allocations and ensure SourceDestCheck is disabled
	r, _ := router.NewAwsRouter(svc)
	for _, nia := range nias {
		r.PreventSourceDestCheck(nia.NatInstance)
		for _, rt := range nia.RoutingTables {
			r.UpsertNatRoute("0.0.0.0/0", nia.NatInstance, rt)
		}
	}

	// TODO: start recovery for unhealthy instances (just issue command... on next iteration Nat Instances will be re-evaluated)
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
