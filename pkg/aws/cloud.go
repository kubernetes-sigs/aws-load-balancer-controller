package aws

import (
	"context"
	"github.com/aws/aws-app-mesh-controller-for-k8s/pkg/aws/metrics"
	"github.com/aws/aws-app-mesh-controller-for-k8s/pkg/aws/services"
	"github.com/aws/aws-app-mesh-controller-for-k8s/pkg/aws/throttle"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
)

type Cloud interface {
	// AppMesh provides API to AWS AppMesh
	AppMesh() services.AppMesh
	// CloudMap provides API to AWS CloudMap
	CloudMap() services.CloudMap

	// AccountID provides AccountID for the kubernetes cluster
	AccountID() string

	// Region for the kubernetes cluster
	Region() string
}

// NewCloud constructs new Cloud implementation.
func NewCloud(cfg CloudConfig, metricsRegisterer prometheus.Registerer) (Cloud, error) {
	sess := session.Must(session.NewSession(aws.NewConfig()))
	injectUserAgent(&sess.Handlers)
	if cfg.ThrottleConfig != nil {
		throttler := throttle.NewThrottler(cfg.ThrottleConfig)
		throttler.InjectHandlers(&sess.Handlers)
	}
	if metricsRegisterer != nil {
		metricsCollector, err := metrics.NewCollector(metricsRegisterer)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize sdk metrics collector")
		}
		metricsCollector.InjectHandlers(&sess.Handlers)
	}

	if len(cfg.Region) == 0 {
		metadata := services.NewEC2Metadata(sess)
		region, err := metadata.Region()
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect region from EC2Metadata, specify --aws-region instead if EC2Metadata is unavailable")
		}
		cfg.Region = region
	}

	awsCfg := aws.NewConfig().WithRegion(cfg.Region).WithSTSRegionalEndpoint(endpoints.RegionalSTSEndpoint)
	sess = sess.Copy(awsCfg)
	if len(cfg.AccountID) == 0 {
		sts := services.NewSTS(sess)
		accountID, err := sts.AccountID(context.Background())
		if err != nil {
			return nil, errors.Wrap(err, "failed to introspect accountID from STS, specify --aws-account-id instead if STS is unavailable")
		}
		cfg.AccountID = accountID
	}
	return &defaultCloud{
		cfg:      cfg,
		appMesh:  services.NewAppMesh(sess),
		cloudMap: services.NewCloudMap(sess),
	}, nil
}

var _ Cloud = &defaultCloud{}

type defaultCloud struct {
	cfg CloudConfig

	appMesh  services.AppMesh
	cloudMap services.CloudMap
}

func (c *defaultCloud) AppMesh() services.AppMesh {
	return c.appMesh
}

func (c *defaultCloud) CloudMap() services.CloudMap {
	return c.cloudMap
}

func (c *defaultCloud) AccountID() string {
	return c.cfg.AccountID
}

func (c *defaultCloud) Region() string {
	return c.cfg.Region
}
