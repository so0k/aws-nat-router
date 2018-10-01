package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/so0k/aws-nat-router/pkg/discover"
	"github.com/so0k/aws-nat-router/pkg/healthcheck"
	"github.com/so0k/aws-nat-router/pkg/router"
	"github.com/urfave/cli"
)

var build = "4"

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
			Usage:  "Required `ID` of the VPC the NAT Instances live in",
			EnvVar: "NAT_VPC_ID",
		},
		cli.StringFlag{
			Name:   "cluster-id",
			Value:  "squid",
			Usage:  "`ID` the NAT Instances are tagged with",
			EnvVar: "NAT_CLUSTER_ID",
		},
		cli.DurationFlag{
			Name:   "interval",
			Value:  10 * time.Second,
			Usage:  "`DURATION` Interval for evaluating NAT Instances and updating routes",
			EnvVar: "NAT_INTERVAL",
		},
		cli.BoolFlag{
			Name:   "ec2-election",
			Usage:  "Use EC2 metadata leader election",
			EnvVar: "NAT_EC2_ELECTION",
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
	awsRoleARN   string
	region       string
	vpcId        string
	clusterId    string
	ec2Election  bool
	instanceId   string
	public       bool
	port         int
	timeout      time.Duration
	interval     time.Duration
}

func parseConfig(c *cli.Context) (*config, error) {
	conf := &config{
		awsSecretKey: c.String("aws-secret-key"),
		awsAccessKey: c.String("aws-access-key"),
		awsRoleARN:   c.String("aws-role-arn"),
		region:       c.String("region"),
		vpcId:        c.String("vpc-id"),
		clusterId:    c.String("cluster-id"),
		ec2Election:  c.Bool("ec2-election"),
		interval:     c.Duration("interval"),
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

	if conf.interval < time.Second {
		return nil, errors.New("Interval should not be less than 1 second")
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
	session := session.New(conf)

	i, err := discover.NewAwsIdentifierFromSession(session)
	if err != nil {
		return err
	}
	appConf.instanceId, err = i.GetIdentity()
	if err != nil && appConf.ec2Election {
		log.Error("EC2 Election requested but not possible, disable --ec2-election")
		log.Error(err)
		cli.ShowAppHelpAndExit(c, 1)
	}

	rc := &RouteController{
		config:  appConf,
		session: session,
	}

	// start control loop
	return rc.Run()
}

type RouteController struct {
	config  *config
	session *session.Session
}

func (c *RouteController) Run() error {
	for {
		err := c.RunOnce()
		if err != nil {
			log.Warnf("Error updating routes: %v", err)
		}
		time.Sleep(c.config.interval)
	}
}

func (c *RouteController) RunOnce() error {
	log.Info("Reconciliation started")
	f, err := discover.NewAwsFinderFromSession(c.session)
	if err != nil {
		return err
	}
	nis, err := f.FindNatInstances(c.config.clusterId, c.config.vpcId)
	if err != nil {
		return err
	}

	// Check liveness for each instance
	var liveNis, deadNis []*discover.NatInstance
	for _, ni := range nis {
		var addr string
		if c.config.public {
			addr = fmt.Sprintf("%v:%v", ni.PublicIP, c.config.port)
		} else {
			addr = fmt.Sprintf("%v:%v", ni.PrivateIP, c.config.port)
		}
		err := healthcheck.TCPCheck(addr, c.config.timeout)
		if err != nil {
			log.Debugf("Instance %q (%v) is dead :(", ni.Id, addr)
			log.Debugf("\tError for TCPCheck: %v", err)
			deadNis = append(deadNis, ni)
		} else {
			log.Debugf("Instance %q (%v) is alive!", ni.Id, addr)
			liveNis = append(liveNis, ni)
			// sorted by LaunchTime
			sort.Slice(liveNis, func(i, j int) bool {
				return liveNis[i].LaunchTime.Before(liveNis[j].LaunchTime)
			})
		}
	}

	log.Infof("Healthy NAT Instances found: %v", len(liveNis))
	if len(liveNis) > 0 && (!c.config.ec2Election || liveNis[0].Id == c.config.instanceId) {
		log.Info("ACTIVE")
		rts, _ := f.FindRoutingTables(c.config.clusterId, c.config.vpcId)

		// Rebuild allocation based on discovered information
		oldNias := router.GetCurrentAllocation(liveNis, rts)

		// Allocate routes to live NATInstances
		newNias := router.AllocateRoutes(liveNis, rts)

		// Verify if allocation differs to avoid exceeding API rate limits
		if router.AllocationDiffers(oldNias, newNias) {
			log.Info("Updating Routes and Source Destination Checks ... ")
			// Apply Allocations and ensure SourceDestCheck is disabled
			r, _ := router.NewAwsRouterFromSession(c.session)
			for _, nia := range newNias {
				r.PreventSourceDestCheck(nia.NatInstance)
				for _, rt := range nia.RoutingTables {
					// hardcoding egress = 0.0.0.0/0
					r.UpsertNatRoute("0.0.0.0/0", nia.NatInstance, rt)
				}
			}
		} else {
			log.Info("Routes are already up to date")
		}
	} else {
		log.Info("PASSIVE")
	}
	// TODO(so0k): start recovery for unhealthy instances (just issue command... on next iteration Nat Instances will be re-evaluated)
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
		&ec2rolecreds.EC2RoleProvider{
			Client: ec2metadata.New(session.New()),
		},
	})
	awsConfig.WithCredentials(creds)
	awsConfig.WithRegion(region)
	return awsConfig
}
