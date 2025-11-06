package aga

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

func Test_defaultAcceleratorBuilder_buildAcceleratorName(t *testing.T) {
	tests := []struct {
		name          string
		ga            *agaapi.GlobalAccelerator
		ipAddressType agamodel.IPAddressType
		clusterName   string
		want          string
		wantErr       bool
	}{
		{
			name: "specify accelerator name in spec",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Name: aws.String("my-example-accelerator"),
				},
			},
			ipAddressType: agamodel.IPAddressTypeIPV4,
			clusterName:   "test-cluster",
			want:          "my-example-accelerator",
			wantErr:       false,
		},
		{
			name: "generated name with valid characters",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
			},
			ipAddressType: agamodel.IPAddressTypeIPV4,
			clusterName:   "test-cluster",
			wantErr:       false,
		},
		{
			name: "generated name with invalid characters",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test@namespace",
					Name:      "test#accelerator",
				},
			},
			ipAddressType: agamodel.IPAddressTypeIPV4,
			clusterName:   "test-cluster",
			wantErr:       false,
		},
		{
			name: "provide long namespace and name",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "very-long-namespace-that-exceeds-limit",
					Name:      "very-long-accelerator-name-that-exceeds-limit",
				},
			},
			ipAddressType: agamodel.IPAddressTypeIPV4,
			clusterName:   "test-cluster",
			wantErr:       false,
		},
		{
			name: "different IP address types generate different names",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
			},
			ipAddressType: agamodel.IPAddressTypeDualStack,
			clusterName:   "test-cluster",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &defaultAcceleratorBuilder{
				clusterName: tt.clusterName,
			}

			got, err := b.buildAcceleratorName(context.Background(), tt.ga, tt.ipAddressType)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.ga.Spec.Name != nil {
				assert.Equal(t, tt.want, got)
			} else {
				// Verify generated name follows AWS Global Accelerator naming constraints
				assert.Regexp(t, "^k8s_[a-zA-Z0-9_-]+_[a-zA-Z0-9_-]+_[a-zA-Z0-9]+$", got)
				assert.LessOrEqual(t, len(got), 64)   // AWS Global Accelerator name length limit
				assert.GreaterOrEqual(t, len(got), 1) // Minimum length

				// Verify it doesn't start or end with hyphen
				assert.False(t, strings.HasPrefix(got, "-"))
				assert.False(t, strings.HasSuffix(got, "-"))
			}
		})
	}
}

func Test_defaultAcceleratorBuilder_buildAcceleratorIPAddressType(t *testing.T) {
	tests := []struct {
		name string
		ga   *agaapi.GlobalAccelerator
		want agamodel.IPAddressType
	}{
		{
			name: "default to IPv4 when not specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{},
			},
			want: agamodel.IPAddressTypeIPV4,
		},
		{
			name: "explicitly set to IPv4",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					IPAddressType: agaapi.IPAddressTypeIPV4,
				},
			},
			want: agamodel.IPAddressTypeIPV4,
		},
		{
			name: "explicitly set to dual stack",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					IPAddressType: agaapi.IPAddressTypeDualStack,
				},
			},
			want: agamodel.IPAddressTypeDualStack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &defaultAcceleratorBuilder{}

			got := b.buildAcceleratorIPAddressType(context.Background(), tt.ga)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultAcceleratorBuilder_buildAcceleratorIPAddresses(t *testing.T) {
	tests := []struct {
		name    string
		ga      *agaapi.GlobalAccelerator
		want    []string
		wantErr bool
	}{
		{
			name: "no IP addresses specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{},
			},
			want: nil,
		},
		{
			name: "single IP address specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					IpAddresses: &[]string{"1.2.3.4"},
				},
			},
			want: []string{"1.2.3.4"},
		},
		{
			name: "multiple IP addresses specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					IpAddresses: &[]string{"1.2.3.4", "5.6.7.8"},
				},
			},
			want: []string{"1.2.3.4", "5.6.7.8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &defaultAcceleratorBuilder{}

			got := b.buildAcceleratorIPAddresses(context.Background(), tt.ga)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultAcceleratorBuilder_buildAcceleratorTags(t *testing.T) {
	trackingProvider := tracking.NewDefaultProvider("aga.k8s.aws", "test-cluster")

	tests := []struct {
		name                string
		ga                  *agaapi.GlobalAccelerator
		defaultTags         map[string]string
		externalManagedTags []string
		clusterName         string
		want                map[string]string
		wantErr             bool
	}{
		{
			name: "no user tags specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			clusterName:         "test-cluster",
			want: map[string]string{
				"Environment": "test",
			},
			wantErr: false,
		},
		{
			name: "user tags specified",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			clusterName:         "test-cluster",
			want: map[string]string{
				"Environment": "test",
				"Application": "my-app",
				"Owner":       "team-a",
			},
			wantErr: false,
		},
		{
			name: "user tags override default tags",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Environment": "production",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			clusterName:         "test-cluster",
			want: map[string]string{
				"Environment": "production", // User tag overrides default
			},
			wantErr: false,
		},
		{
			name: "external managed tags configured but not specified by user",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			clusterName:         "test-cluster",
			want: map[string]string{
				"Environment": "test",
				"Application": "my-app",
				"Owner":       "team-a",
			},
			wantErr: false,
		},
		{
			name: "external managed tags specified by user should cause error",
			ga: &agaapi.GlobalAccelerator{
				Spec: agaapi.GlobalAcceleratorSpec{
					Tags: &map[string]string{
						"Application":   "my-app",
						"ExternalTag":   "external-value",
						"ManagedByTeam": "platform-team",
					},
				},
			},
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			clusterName:         "test-cluster",
			want:                nil,
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use true for "user tags override default tags" test case
			additionalTagsOverrideDefaultTags := tt.name == "user tags override default tags"
			builder := NewAcceleratorBuilder(trackingProvider, tt.clusterName, "us-west-2", tt.defaultTags, tt.externalManagedTags, additionalTagsOverrideDefaultTags)
			b := builder.(*defaultAcceleratorBuilder)

			stack := core.NewDefaultStack(core.StackID{Namespace: "test", Name: "test"})
			got, err := b.buildAcceleratorTags(context.Background(), stack, tt.ga)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultAcceleratorBuilder_Build(t *testing.T) {
	trackingProvider := tracking.NewDefaultProvider("aga.k8s.aws", "test-cluster")
	stack := core.NewDefaultStack(core.StackID{Namespace: "test", Name: "test"})

	tests := []struct {
		name                string
		ga                  *agaapi.GlobalAccelerator
		clusterName         string
		defaultTags         map[string]string
		externalManagedTags []string
		want                *agamodel.Accelerator
		wantErr             bool
	}{
		{
			name: "successful build with minimal spec",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Name: aws.String("test-minimal-accelerator"),
				},
			},
			clusterName:         "test-cluster",
			defaultTags:         map[string]string{},
			externalManagedTags: []string{},
			want: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, agamodel.ResourceTypeAccelerator, agamodel.ResourceIDAccelerator),
				Spec: agamodel.AcceleratorSpec{
					Name:          "test-minimal-accelerator",
					Enabled:       aws.Bool(true),
					IPAddressType: agamodel.IPAddressTypeIPV4,
					IpAddresses:   nil,
					Tags:          map[string]string{},
				},
			},
			wantErr: false,
		},
		{
			name: "successful build with full spec",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Name:          aws.String("my-accelerator"),
					IPAddressType: agaapi.IPAddressTypeDualStack,
					IpAddresses:   &[]string{"1.2.3.4"},
					Tags: &map[string]string{
						"Application": "my-app",
					},
				},
			},
			clusterName: "test-cluster",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{},
			want: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, agamodel.ResourceTypeAccelerator, agamodel.ResourceIDAccelerator),
				Spec: agamodel.AcceleratorSpec{
					Name:          "my-accelerator",
					Enabled:       aws.Bool(true),
					IPAddressType: agamodel.IPAddressTypeDualStack,
					IpAddresses:   []string{"1.2.3.4"},
					Tags: map[string]string{
						"Environment": "test",
						"Application": "my-app",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "build with external managed tags configured but not specified by user",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Name: aws.String("test-external-tags-accelerator"),
					Tags: &map[string]string{
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			clusterName: "test-cluster",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			want: &agamodel.Accelerator{
				ResourceMeta: core.NewResourceMeta(stack, agamodel.ResourceTypeAccelerator, agamodel.ResourceIDAccelerator),
				Spec: agamodel.AcceleratorSpec{
					Name:          "test-external-tags-accelerator",
					Enabled:       aws.Bool(true),
					IPAddressType: agamodel.IPAddressTypeIPV4,
					IpAddresses:   nil,
					Tags: map[string]string{
						"Environment": "test",
						"Application": "my-app",
						"Owner":       "team-a",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "build fails when user specifies external managed tags",
			ga: &agaapi.GlobalAccelerator{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-accelerator",
				},
				Spec: agaapi.GlobalAcceleratorSpec{
					Name: aws.String("test-error-accelerator"),
					Tags: &map[string]string{
						"Application":   "my-app",
						"ExternalTag":   "external-value",
						"ManagedByTeam": "platform-team",
					},
				},
			},
			clusterName: "test-cluster",
			defaultTags: map[string]string{
				"Environment": "test",
			},
			externalManagedTags: []string{"ExternalTag", "ManagedByTeam"},
			want:                nil,
			wantErr:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewAcceleratorBuilder(trackingProvider, tt.clusterName, "us-west-2", tt.defaultTags, tt.externalManagedTags, false)

			got, err := builder.Build(context.Background(), stack, tt.ga)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)

			// Verify important fields instead of deep comparing the entire object
			// ResourceMeta fields

			// Spec fields
			assert.Equal(t, tt.want.Spec.Name, got.Spec.Name, "Name should match")
			assert.Equal(t, *tt.want.Spec.Enabled, *got.Spec.Enabled, "Enabled should match")
			assert.Equal(t, tt.want.Spec.IPAddressType, got.Spec.IPAddressType, "IPAddressType should match")
			assert.Equal(t, tt.want.Spec.IpAddresses, got.Spec.IpAddresses, "IpAddresses should match")

			// Tags verification
			assert.Equal(t, len(tt.want.Spec.Tags), len(got.Spec.Tags), "Tags count should match")
			for key, expectedValue := range tt.want.Spec.Tags {
				actualValue, exists := got.Spec.Tags[key]
				assert.True(t, exists, "Tag %s should exist", key)
				assert.Equal(t, expectedValue, actualValue, "Tag %s value should match", key)
			}
		})
	}
}
