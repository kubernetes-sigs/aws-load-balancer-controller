package tracking

import (
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
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
				"elbv2.k8s.aws/cluster": "cluster-name",
				"ingress.k8s.aws/stack": "awesome-group",
			},
		},
		{
			name:     "stackTags for implicit IngressGroup",
			provider: NewDefaultProvider("ingress.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})},
			want: map[string]string{
				"elbv2.k8s.aws/cluster": "cluster-name",
				"ingress.k8s.aws/stack": "namespace/ingressName",
			},
		},
		{
			name:     "stackTags for Service",
			provider: NewDefaultProvider("service.k8s.aws", "cluster-name"),
			args:     args{stack: core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "serviceName"})},
			want: map[string]string{
				"elbv2.k8s.aws/cluster": "cluster-name",
				"service.k8s.aws/stack": "namespace/serviceName",
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
	stack := core.NewDefaultStack(core.StackID{Namespace: "namespace", Name: "ingressName"})
	fakeRes := core.NewFakeResource(stack, "fake", "fake-id", core.FakeResourceSpec{}, nil)

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
				stack: stack,
				res:   fakeRes,
			},
			want: map[string]string{
				"elbv2.k8s.aws/cluster":    "cluster-name",
				"ingress.k8s.aws/stack":    "namespace/ingressName",
				"ingress.k8s.aws/resource": "fake-id",
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
