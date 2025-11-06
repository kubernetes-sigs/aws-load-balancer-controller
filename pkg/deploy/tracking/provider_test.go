package tracking

import (
	"github.com/stretchr/testify/assert"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/shared_constants"
	"testing"
)

func Test_defaultProvider_ResourceIDTagKey(t *testing.T) {
	tests := []struct {
		name     string
		provider *defaultProvider
		want     string
	}{
		{
			name:     "resourceTagKey for Ingress",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			want:     "ingress.k8s.aws/resource",
		},
		{
			name:     "resourceTagKey for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			want:     "service.k8s.aws/resource",
		},
		{
			name:     "resourceTagKey for AGA",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name"),
			want:     "aga.k8s.aws/resource",
		},
		{
			name:     "resourceTagKey for Gateway",
			provider: NewDefaultProvider("gateway.k8s.aws", "cluster-name"),
			want:     "gateway.k8s.aws/resource",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.ResourceIDTagKey()
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_StackTags(t *testing.T) {
	type args struct {
		stack core.Stack
	}
	tests := []struct {
		name     string
		provider *defaultProvider
		args     args
		want     map[string]string
	}{
		{
			name:     "stackTags for explicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "", Name: "awesome-group"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"ingress.k8s.aws/stack":           "awesome-group",
			},
		},
		{
			name:     "stackTags for implicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"ingress.k8s.aws/stack":           "namespace/ingressName",
			},
		},
		{
			name:     "stackTags for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"service.k8s.aws/stack":           "namespace/serviceName",
			},
		},
		{
			name:     "stackTags for AGA",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "globalAcceleratorName"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"aga.k8s.aws/stack":               "namespace/globalAcceleratorName",
			},
		},
		{
			name:     "stackTags for Gateway",
			provider: NewDefaultProvider("gateway.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "gatewayName"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"gateway.k8s.aws/stack":           "namespace/gatewayName",
			},
		},
		{
			name:     "stackTags for AGA with region",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name", WithRegion("us-west-2")),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "globalAcceleratorName"})},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"aga.k8s.aws/stack":               "namespace/globalAcceleratorName",
				"elbv2.k8s.aws/cluster-region":    "us-west-2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.StackTags(tt.args.stack)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_ResourceTags(t *testing.T) {
	ingressStack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})
	ingressFakeRes := core.NewFakeResource(ingressStack, "fake", "fake-id", core.FakeResourceSpec{}, nil)

	serviceStack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})
	serviceFakeRes := core.NewFakeResource(serviceStack, "fake", "service-id", core.FakeResourceSpec{}, nil)

	agaStack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "globalAcceleratorName"})
	agaFakeRes := core.NewFakeResource(agaStack, "fake", agamodel.ResourceIDAccelerator, core.FakeResourceSpec{}, nil)

	gatewayStack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "gatewayName"})
	gatewayFakeRes := core.NewFakeResource(gatewayStack, "fake", "gateway-id", core.FakeResourceSpec{}, nil)

	type args struct {
		stack          core.Stack
		res            core.Resource
		additionalTags map[string]string
	}
	tests := []struct {
		name     string
		provider *defaultProvider
		args     args
		want     map[string]string
	}{
		{
			name:     "resourceTags for Ingress",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args: args{
				stack: ingressStack,
				res:   ingressFakeRes,
			},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"ingress.k8s.aws/stack":           "namespace/ingressName",
				"ingress.k8s.aws/resource":        "fake-id",
			},
		},
		{
			name:     "resourceTags for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args: args{
				stack: serviceStack,
				res:   serviceFakeRes,
			},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"service.k8s.aws/stack":           "namespace/serviceName",
				"service.k8s.aws/resource":        "service-id",
			},
		},
		{
			name:     "resourceTags for AGA",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name"),
			args: args{
				stack: agaStack,
				res:   agaFakeRes,
			},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"aga.k8s.aws/stack":               "namespace/globalAcceleratorName",
				"aga.k8s.aws/resource":            "GlobalAccelerator",
			},
		},
		{
			name:     "resourceTags for Gateway",
			provider: NewDefaultProvider("gateway.k8s.aws", "cluster-name"),
			args: args{
				stack: gatewayStack,
				res:   gatewayFakeRes,
			},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"gateway.k8s.aws/stack":           "namespace/gatewayName",
				"gateway.k8s.aws/resource":        "gateway-id",
			},
		},
		{
			name:     "resourceTags for AGA with region",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name", WithRegion("us-east-1")),
			args: args{
				stack: agaStack,
				res:   agaFakeRes,
			},
			want: map[string]string{
				shared_constants.TagKeyK8sCluster: "cluster-name",
				"aga.k8s.aws/stack":               "namespace/globalAcceleratorName",
				"aga.k8s.aws/resource":            "GlobalAccelerator",
				"elbv2.k8s.aws/cluster-region":    "us-east-1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.ResourceTags(tt.args.stack, tt.args.res, tt.args.additionalTags)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_StackLabels(t *testing.T) {
	type args struct {
		stack core.Stack
	}
	tests := []struct {
		name     string
		provider *defaultProvider
		args     args
		want     map[string]string
	}{
		{
			name:     "stackLabels for explicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "", Name: "awesome-group"})},
			want: map[string]string{
				"ingress.k8s.aws/stack": "awesome-group",
			},
		},
		{
			name:     "stackLabels for implicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})},
			want: map[string]string{
				"ingress.k8s.aws/stack-namespace": "namespace",
				"ingress.k8s.aws/stack-name":      "ingressName",
			},
		},
		{
			name:     "stackLabels for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			want: map[string]string{
				"service.k8s.aws/stack-namespace": "namespace",
				"service.k8s.aws/stack-name":      "serviceName",
			},
		},
		{
			name:     "stackLabels for AGA",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "globalAcceleratorName"})},
			want: map[string]string{
				"aga.k8s.aws/stack-namespace": "namespace",
				"aga.k8s.aws/stack-name":      "globalAcceleratorName",
			},
		},
		{
			name:     "stackLabels for Gateway",
			provider: NewDefaultProvider("gateway.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "gatewayName"})},
			want: map[string]string{
				"gateway.k8s.aws/stack-namespace": "namespace",
				"gateway.k8s.aws/stack-name":      "gatewayName",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.StackLabels(tt.args.stack)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_StackTagsLegacy(t *testing.T) {
	type args struct {
		stack core.Stack
	}
	tests := []struct {
		name     string
		provider *defaultProvider
		args     args
		want     map[string]string
	}{
		{
			name:     "stackTags for explicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "", Name: "awesome-group"})},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"ingress.k8s.aws/stack":   "awesome-group",
			},
		},
		{
			name:     "stackTags for implicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"ingress.k8s.aws/stack":   "namespace/ingressName",
			},
		},
		{
			name:     "stackTags for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"service.k8s.aws/stack":   "namespace/serviceName",
			},
		},
		{
			name:     "stackTags for AGA",
			provider: NewDefaultProvider("aga.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "globalAcceleratorName"})},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"aga.k8s.aws/stack":       "namespace/globalAcceleratorName",
			},
		},
		{
			name:     "stackTags for Gateway",
			provider: NewDefaultProvider("gateway.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "gatewayName"})},
			want: map[string]string{
				"ingress.k8s.aws/cluster": "cluster-name",
				"gateway.k8s.aws/stack":   "namespace/gatewayName",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.StackTagsLegacy(tt.args.stack)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultProvider_LegacyTagKeys(t *testing.T) {
	type fields struct {
		clusterName string
	}
	tests := []struct {
		name   string
		fields fields
		want   []string
	}{
		{
			name: "standard case",
			fields: fields{
				clusterName: "my-cluster",
			},
			want: []string{
				"kubernetes.io/cluster/my-cluster",
				"kubernetes.io/cluster-name",
				"kubernetes.io/namespace",
				"kubernetes.io/ingress-name",
				"kubernetes.io/service-name",
				"kubernetes.io/service-port",
				"ingress.k8s.aws/cluster",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &defaultProvider{
				clusterName: tt.fields.clusterName,
			}
			got := p.LegacyTagKeys()
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_WithRegion(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected *string
	}{
		{
			name:     "WithRegion sets region",
			region:   "us-west-2",
			expected: func() *string { s := "us-west-2"; return &s }(),
		},
		{
			name:     "WithRegion sets empty region",
			region:   "",
			expected: func() *string { s := ""; return &s }(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewDefaultProvider("aga.k8s.aws", "cluster-name", WithRegion(tt.region))
			if tt.expected == nil {
				assert.Nil(t, provider.region)
			} else {
				assert.NotNil(t, provider.region)
				assert.Equal(t, *tt.expected, *provider.region)
			}
		})
	}
}
