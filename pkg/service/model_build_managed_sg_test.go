package service

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/config"
	ec2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/ec2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func Test_buildCIDRsFromSourceRanges_buildCIDRsFromSourceRanges(t *testing.T) {
	type fields struct {
		svc                   *corev1.Service
		ipAddressType         elbv2model.IPAddressType
		prefixListsConfigured bool
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr bool
	}{
		{
			name: "default IPv4",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: false,
			},
			wantErr: false,
			want: []string{
				"0.0.0.0/0",
			},
		},
		{
			name: "default dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: false,
			},
			wantErr: false,
			want: []string{
				"0.0.0.0/0",
				"::/0",
			},
		},
		{
			name: "no ip range but prefix list",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeDualStack,
				prefixListsConfigured: true,
			},
			wantErr: false,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
			}
			got, err := task.buildCIDRsFromSourceRanges(context.Background(), tt.fields.ipAddressType, tt.fields.prefixListsConfigured)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_buildCIDRsFromSourceRanges_buildManagedSecurityGroupIngressPermissions(t *testing.T) {
	type fields struct {
		svc           *corev1.Service
		ipAddressType elbv2model.IPAddressType
	}
	tests := []struct {
		name    string
		fields  fields
		want    []ec2model.IPPermission
		wantErr bool
	}{
		{
			name: "default IPv4",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
			},
		},
		{
			name: "default dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-ip-address-type": "dualstack",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "::/0",
						},
					},
				},
			},
		},
		{
			name: "no ip range but prefix list",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-security-group-prefix-lists": "pl-xxxxx",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-xxxxx",
						},
					},
				},
			},
		},
		{
			name: "no ip range but multiple prefix lists",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-security-group-prefix-lists": "pl-xxxxx, pl-yyyyyy",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-xxxxx",
						},
					},
				},
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					PrefixLists: []ec2model.PrefixList{
						{
							ListID: "pl-yyyyyy",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
			}
			got, err := task.buildManagedSecurityGroupIngressPermissions(context.Background(), tt.fields.ipAddressType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildManagedSecurityGroupTags(t *testing.T) {
	tests := []struct {
		name                string
		enabledFeatureGates func() config.FeatureGates
		defaultTags         map[string]string
		svc                 *corev1.Service
		wantTags            map[string]string
		wantErr             bool
	}{
		{
			name:                "no default tags, no annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags:         map[string]string{},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantTags: map[string]string{},
			wantErr:  false,
		},
		{
			name:                "no default tags, annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags:         map[string]string{},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
			},
			wantErr: false,
		},
		{
			name:                "default tags, no annotation tags",
			enabledFeatureGates: func() config.FeatureGates { return config.NewFeatureGates() },
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			wantTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			wantErr: false,
		},
		{
			name: "default tags, annotation tags, collision where default tags take priority",
			enabledFeatureGates: func() config.FeatureGates {
				featureGates := config.NewFeatureGates()
				featureGates.Disable(config.EnableDefaultTagsLowPriority)
				return featureGates
			},
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
				"k3": "v3",
			},
			wantErr: false,
		},
		{
			name: "default tags, annotation tags, collision where annotation tags take priority",
			enabledFeatureGates: func() config.FeatureGates {
				featureGates := config.NewFeatureGates()
				featureGates.Enable(config.EnableDefaultTagsLowPriority)
				return featureGates
			},
			defaultTags: map[string]string{
				"k1": "v10",
				"k2": "v20",
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags": "k1=v1,k2=v2,k3=v3",
					},
				},
			},
			wantTags: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				featureGates:     tt.enabledFeatureGates(),
				defaultTags:      tt.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
				service:          tt.svc,
			}
			got, err := task.buildManagedSecurityGroupTags(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTags, got)
			}
			for key, value := range tt.wantTags {
				assert.Contains(t, got, key)
				assert.Equal(t, value, got[key])
			}
		})
	}
}

func Test_buildExplicitOutboundCIDRs_buildExplicitOutboundCIDRs(t *testing.T) {
	type fields struct {
		svc                   *corev1.Service
		ipAddressType         elbv2model.IPAddressType
		prefixListsConfigured bool
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr bool
	}{
		{
			name: "no annotation configured, nil slice returned",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: false,
			},
			wantErr: false,
			want:    nil,
		},
		{
			name: "empty annotation configured, empty slice returned",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: false,
			},
			wantErr: false,
			want:    []string{},
		},
		{
			name: "configured with single valid ipv4 value",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "0.0.0.0/0",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: true,
			},
			wantErr: false,
			want: []string{
				"0.0.0.0/0",
			},
		},
		{
			name: "configured with invalid cidr",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "10.0.0.0",
						},
					},
				},
				ipAddressType:         elbv2model.IPAddressTypeIPV4,
				prefixListsConfigured: true,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
			}
			got, err := task.buildExplicitOutboundCIDRs(context.Background(), tt.fields.ipAddressType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_buildManagedSecurityGroupEgressPermissions(t *testing.T) {
	type fields struct {
		svc           *corev1.Service
		ipAddressType elbv2model.IPAddressType
	}
	tests := []struct {
		name    string
		fields  fields
		want    []ec2model.IPPermission
		wantErr bool
	}{
		{
			name: "no annotation configured, nil slice returned",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: false,
			want:    nil,
		},
		{
			name: "empty annotation configured, empty slice returned",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: false,
			want:    []ec2model.IPPermission{},
		},
		{
			name: "single valid ipv4",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "0.0.0.0/0",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
			},
		},
		{
			name: "multiple valid ipv4 and ipv6 with dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "0.0.0.0/0,::/0",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeDualStack,
			},
			wantErr: false,
			want: []ec2model.IPPermission{
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPRanges: []ec2model.IPRange{
						{
							CIDRIP: "0.0.0.0/0",
						},
					},
				},
				{
					IPProtocol: "",
					FromPort:   aws.Int32(80),
					ToPort:     aws.Int32(80),
					IPv6Range: []ec2model.IPv6Range{
						{
							CIDRIPv6: "::/0",
						},
					},
				},
			},
		},
		{
			name: "multiple valid ipv4 and ipv6 no dualstack",
			fields: fields{
				svc: &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"service.beta.kubernetes.io/aws-load-balancer-outbound-cidrs": "0.0.0.0/0,::/0",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeNodePort,
						Ports: []corev1.ServicePort{
							{
								Name:     "http",
								Port:     80,
								NodePort: 18080,
							},
						},
						LoadBalancerSourceRanges: []string{},
					},
				},
				ipAddressType: elbv2model.IPAddressTypeIPV4,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io")
			task := &defaultModelBuildTask{
				annotationParser: annotationParser,
				service:          tt.fields.svc,
			}
			got, err := task.buildManagedSecurityGroupEgressPermissions(context.Background(), tt.fields.ipAddressType)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}
