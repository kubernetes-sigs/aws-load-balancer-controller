package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/cache"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/provider"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/aws/services"
	ctrl "sigs.k8s.io/controller-runtime"
)

func Test_getVpcID(t *testing.T) {
	logger := ctrl.Log.WithName("test")

	tests := []struct {
		name       string
		cfg        CloudConfig
		setupMocks func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata)
		wantVpcID  string
		wantErr    string
	}{
		{
			name: "explicit vpc-id takes priority over everything",
			cfg: CloudConfig{
				VpcID:   "vpc-explicit",
				VpcTags: map[string]string{"Name": "my-vpc"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				// no calls expected
			},
			wantVpcID: "vpc-explicit",
		},
		{
			name: "tags lookup uses all tags as filters",
			cfg: CloudConfig{
				VpcTags: map[string]string{"Name": "my-vpc"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), &ec2.DescribeVpcsInput{
					Filters: []ec2types.Filter{
						{Name: aws.String("tag:Name"), Values: []string{"my-vpc"}},
					},
				}).Return([]ec2types.Vpc{
					{VpcId: aws.String("vpc-from-name-tag")},
				}, nil)
			},
			wantVpcID: "vpc-from-name-tag",
		},
		{
			name: "tags lookup with multiple tags uses all as AND filters",
			cfg: CloudConfig{
				VpcTags: map[string]string{"foo": "bar", "baz": "buzz"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, input *ec2.DescribeVpcsInput) ([]ec2types.Vpc, error) {
						assert.Len(t, input.Filters, 2)
						filterNames := map[string]string{}
						for _, f := range input.Filters {
							filterNames[*f.Name] = f.Values[0]
						}
						assert.Equal(t, "bar", filterNames["tag:foo"])
						assert.Equal(t, "buzz", filterNames["tag:baz"])
						return []ec2types.Vpc{
							{VpcId: aws.String("vpc-multi-tag")},
						}, nil
					})
			},
			wantVpcID: "vpc-multi-tag",
		},
		{
			name: "tags lookup with no matching VPCs returns error",
			cfg: CloudConfig{
				VpcTags: map[string]string{"foo": "bar", "baz": "buzz"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					Return([]ec2types.Vpc{}, nil)
			},
			wantErr: "no VPC exists with tags",
		},
		{
			name: "tags lookup with multiple matching VPCs returns error",
			cfg: CloudConfig{
				VpcTags: map[string]string{"Env": "prod"},
			},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Service.EXPECT().DescribeVPCsAsList(gomock.Any(), gomock.Any()).
					Return([]ec2types.Vpc{
						{VpcId: aws.String("vpc-1")},
						{VpcId: aws.String("vpc-2")},
					}, nil)
			},
			wantErr: "multiple VPCs exist with tags",
		},
		{
			name: "no vpc-id and no tags falls back to IMDS",
			cfg:  CloudConfig{},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Metadata.EXPECT().VpcID().Return("vpc-from-imds", nil)
			},
			wantVpcID: "vpc-from-imds",
		},
		{
			name: "IMDS fallback failure returns error",
			cfg:  CloudConfig{},
			setupMocks: func(ec2Service *services.MockEC2, ec2Metadata *services.MockEC2Metadata) {
				ec2Metadata.EXPECT().VpcID().Return("", fmt.Errorf("IMDS unavailable"))
			},
			wantErr: "failed to fetch VPC ID from instance metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ec2Service := services.NewMockEC2(ctrl)
			ec2Metadata := services.NewMockEC2Metadata(ctrl)
			tt.setupMocks(ec2Service, ec2Metadata)

			got, err := getVpcID(tt.cfg, ec2Service, ec2Metadata, logger)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantVpcID, got)
			}
		})
	}
}

func Test_assumedRoleCacheKey(t *testing.T) {
	tests := []struct {
		name string
		a    assumedRoleCacheKey
		b    assumedRoleCacheKey
		same bool
	}{
		{
			name: "identical keys are equal",
			a:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: "ext-abc"},
			b:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: "ext-abc"},
			same: true,
		},
		{
			name: "same ARN different externalId are not equal",
			a:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/Shared", externalId: "tenant-A"},
			b:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/Shared", externalId: "tenant-B"},
			same: false,
		},
		{
			name: "different ARN same externalId are not equal",
			a:    assumedRoleCacheKey{roleArn: "arn:aws:iam::111111111111:role/RoleA", externalId: "ext-1"},
			b:    assumedRoleCacheKey{roleArn: "arn:aws:iam::222222222222:role/RoleB", externalId: "ext-1"},
			same: false,
		},
		{
			name: "empty externalId matches empty externalId",
			a:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: ""},
			b:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: ""},
			same: true,
		},
		{
			name: "empty vs non-empty externalId are not equal",
			a:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: ""},
			b:    assumedRoleCacheKey{roleArn: "arn:aws:iam::123456789012:role/MyRole", externalId: "ext-1"},
			same: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.same {
				assert.Equal(t, tt.a, tt.b)
			} else {
				assert.NotEqual(t, tt.a, tt.b)
			}
		})
	}
}

// stubAWSClientsProvider serves a fixed STS client and builds EC2 clients against a fixed endpoint.
// Embedding the interface leaves every other method nil so an unexpected call fails loudly.
type stubAWSClientsProvider struct {
	provider.AWSClientsProvider
	stsClient   *sts.Client
	ec2Endpoint string
}

func (p *stubAWSClientsProvider) GetSTSClient(_ context.Context, _ string) (*sts.Client, error) {
	return p.stsClient, nil
}

func (p *stubAWSClientsProvider) GenerateNewEC2Client(cfg aws.Config) *ec2.Client {
	return ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(p.ec2Endpoint)
	})
}

// stubAWSConfigGenerator applies the supplied options to an otherwise empty config, so the
// credentials handed to it are the only credentials the resulting clients can sign with.
type stubAWSConfigGenerator struct{}

func (stubAWSConfigGenerator) GenerateAWSConfig(optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
	var lo config.LoadOptions
	for _, fn := range optFns {
		if err := fn(&lo); err != nil {
			return aws.Config{}, err
		}
	}
	return aws.Config{Region: "us-west-2", Credentials: lo.Credentials}, nil
}

// fakeAWSQueryServer answers STS AssumeRole and EC2 DescribeAvailabilityZones over the AWS query
// protocol and records what it received.
type fakeAWSQueryServer struct {
	*httptest.Server
	assumedAccessKeyID string
	stsDenied          bool

	mu                sync.Mutex
	assumeRoleForms   []url.Values
	ec2Authorizations []string
}

func newFakeAWSQueryServer(assumedAccessKeyID string) *fakeAWSQueryServer {
	f := &fakeAWSQueryServer{assumedAccessKeyID: assumedAccessKeyID}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeAWSQueryServer) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml")
	switch r.Form.Get("Action") {
	case "AssumeRole":
		f.assumeRoleForms = append(f.assumeRoleForms, r.Form)
		if f.stsDenied {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>AccessDenied</Code><Message>not authorized</Message></Error><RequestId>req-1</RequestId></ErrorResponse>`)
			return
		}
		fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>%s</AccessKeyId><SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-token</SessionToken><Expiration>%s</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::111122223333:assumed-role/tg-role/session</Arn><AssumedRoleId>AROAEXAMPLE:session</AssumedRoleId></AssumedRoleUser></AssumeRoleResult><ResponseMetadata><RequestId>req-2</RequestId></ResponseMetadata></AssumeRoleResponse>`,
			f.assumedAccessKeyID, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	case "DescribeAvailabilityZones":
		f.ec2Authorizations = append(f.ec2Authorizations, r.Header.Get("Authorization"))
		fmt.Fprint(w, `<DescribeAvailabilityZonesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>req-3</requestId><availabilityZoneInfo><item><zoneName>us-west-2a</zoneName><zoneId>usw2-az1</zoneId><zoneState>available</zoneState><regionName>us-west-2</regionName></item></availabilityZoneInfo></DescribeAvailabilityZonesResponse>`)
	default:
		http.Error(w, "unexpected action "+r.Form.Get("Action"), http.StatusBadRequest)
	}
}

func (f *fakeAWSQueryServer) snapshot() ([]url.Values, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.assumeRoleForms...), append([]string(nil), f.ec2Authorizations...)
}

func newTestCloudForAssumedRoleEC2(srv *fakeAWSQueryServer, defaultEC2 services.EC2) *defaultCloud {
	stsClient := sts.NewFromConfig(aws.Config{
		Region:      "us-west-2",
		Credentials: credentials.NewStaticCredentialsProvider("CONTROLLERKEY", "controller-secret", ""),
	}, func(o *sts.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
	return &defaultCloud{
		clusterName:        "my-cluster",
		ec2:                defaultEC2,
		assumeRoleEc2Cache: cache.NewExpiring(),
		awsClientsProvider: &stubAWSClientsProvider{stsClient: stsClient, ec2Endpoint: srv.URL},
		awsConfigGenerator: stubAWSConfigGenerator{},
		logger:             ctrl.Log.WithName("test"),
	}
}

func Test_defaultCloud_GetAssumedRoleEC2(t *testing.T) {
	ctx := context.Background()
	const roleArn = "arn:aws:iam::111122223333:role/tg-role"

	t.Run("empty role ARN returns the default EC2 client without calling STS", func(t *testing.T) {
		srv := newFakeAWSQueryServer("ASSUMEDKEY")
		defer srv.Close()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		defaultEC2 := services.NewMockEC2(ctrl)
		c := newTestCloudForAssumedRoleEC2(srv, defaultEC2)

		got, err := c.GetAssumedRoleEC2(ctx, "", "")
		assert.NoError(t, err)
		assert.Same(t, defaultEC2, got)
		assumeRoleForms, _ := srv.snapshot()
		assert.Empty(t, assumeRoleForms)
	})

	t.Run("assumed-role client is built from STS credentials and routes EC2 calls through them", func(t *testing.T) {
		srv := newFakeAWSQueryServer("ASSUMEDKEY")
		defer srv.Close()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		c := newTestCloudForAssumedRoleEC2(srv, services.NewMockEC2(ctrl))

		got, err := c.GetAssumedRoleEC2(ctx, roleArn, "ext-1")
		assert.NoError(t, err)
		assert.NotNil(t, got)

		resp, err := got.DescribeAvailabilityZonesWithContext(ctx, &ec2.DescribeAvailabilityZonesInput{})
		assert.NoError(t, err)
		assert.Equal(t, []ec2types.AvailabilityZone{{
			ZoneName:   aws.String("us-west-2a"),
			ZoneId:     aws.String("usw2-az1"),
			State:      ec2types.AvailabilityZoneStateAvailable,
			RegionName: aws.String("us-west-2"),
		}}, resp.AvailabilityZones)

		assumeRoleForms, ec2Authorizations := srv.snapshot()
		assert.Len(t, assumeRoleForms, 1)
		assert.Equal(t, roleArn, assumeRoleForms[0].Get("RoleArn"))
		assert.Equal(t, "ext-1", assumeRoleForms[0].Get("ExternalId"))
		assert.Equal(t, generateAssumeRoleSessionName("my-cluster"), assumeRoleForms[0].Get("RoleSessionName"))
		assert.Len(t, ec2Authorizations, 1)
		assert.Contains(t, ec2Authorizations[0], "Credential=ASSUMEDKEY/")
		assert.NotContains(t, ec2Authorizations[0], "CONTROLLERKEY")
	})

	t.Run("external ID is omitted from the STS request when empty", func(t *testing.T) {
		srv := newFakeAWSQueryServer("ASSUMEDKEY")
		defer srv.Close()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		c := newTestCloudForAssumedRoleEC2(srv, services.NewMockEC2(ctrl))

		_, err := c.GetAssumedRoleEC2(ctx, roleArn, "")
		assert.NoError(t, err)
		assumeRoleForms, _ := srv.snapshot()
		assert.Len(t, assumeRoleForms, 1)
		assert.False(t, assumeRoleForms[0].Has("ExternalId"))
	})

	t.Run("clients are cached per role ARN and external ID", func(t *testing.T) {
		srv := newFakeAWSQueryServer("ASSUMEDKEY")
		defer srv.Close()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		c := newTestCloudForAssumedRoleEC2(srv, services.NewMockEC2(ctrl))

		first, err := c.GetAssumedRoleEC2(ctx, roleArn, "ext-1")
		assert.NoError(t, err)
		second, err := c.GetAssumedRoleEC2(ctx, roleArn, "ext-1")
		assert.NoError(t, err)
		assert.Same(t, first, second)
		assumeRoleForms, _ := srv.snapshot()
		assert.Len(t, assumeRoleForms, 1)

		third, err := c.GetAssumedRoleEC2(ctx, roleArn, "ext-2")
		assert.NoError(t, err)
		assert.NotSame(t, first, third)
		assumeRoleForms, _ = srv.snapshot()
		assert.Len(t, assumeRoleForms, 2)
	})

	t.Run("STS failure is returned and nothing is cached", func(t *testing.T) {
		srv := newFakeAWSQueryServer("ASSUMEDKEY")
		defer srv.Close()
		srv.stsDenied = true
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		c := newTestCloudForAssumedRoleEC2(srv, services.NewMockEC2(ctrl))

		got, err := c.GetAssumedRoleEC2(ctx, roleArn, "ext-1")
		assert.Nil(t, got)
		assert.ErrorContains(t, err, "AccessDenied")
		_, exists := c.assumeRoleEc2Cache.Get(assumedRoleCacheKey{roleArn: roleArn, externalId: "ext-1"})
		assert.False(t, exists)
	})
}
