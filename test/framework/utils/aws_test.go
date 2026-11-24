package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newNode(name, zone string) *corev1.Node {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if zone != "" {
		node.Labels = map[string]string{"topology.kubernetes.io/zone": zone}
	}
	return node
}

func newNodeClass(name string, subnetZones ...string) *unstructured.Unstructured {
	subnets := make([]interface{}, 0, len(subnetZones))
	for _, zone := range subnetZones {
		subnets = append(subnets, map[string]interface{}{
			"id":   "subnet-" + zone,
			"zone": zone,
		})
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(autoModeNodeClassGVK)
	obj.SetName(name)
	obj.Object["status"] = map[string]interface{}{"subnets": subnets}
	return obj
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func Test_GetClusterZones(t *testing.T) {
	tests := []struct {
		name          string
		objs          []client.Object
		nodeClassName string
		want          []string
		wantErr       bool
	}{
		{
			name: "no NodeClass name uses node zone labels",
			objs: []client.Object{
				newNode("node-1", "us-west-2a"),
				newNode("node-2", "us-west-2b"),
				newNode("node-3", "us-west-2a"),
				newNode("node-no-label", ""),
				newNodeClass("default", "us-west-2a", "us-west-2b", "us-west-2c"),
			},
			nodeClassName: "",
			want:          []string{"us-west-2a", "us-west-2b"},
		},
		{
			name: "NodeClass name uses that NodeClass's subnet zones instead of nodes",
			objs: []client.Object{
				newNode("node-1", "eu-west-1a"),
				newNodeClass("default",
					"eu-west-1c", "eu-west-1a", "eu-west-1c",
					"eu-west-1b", "eu-west-1b", "eu-west-1a"),
			},
			nodeClassName: "default",
			want:          []string{"eu-west-1a", "eu-west-1b", "eu-west-1c"},
		},
		{
			name: "NodeClass name ignores other NodeClasses",
			objs: []client.Object{
				newNodeClass("default", "us-east-1a"),
				newNodeClass("custom", "us-east-1b"),
			},
			nodeClassName: "custom",
			want:          []string{"us-east-1b"},
		},
		{
			name: "NodeClass name errors when the NodeClass does not exist",
			objs: []client.Object{
				newNode("node-1", "us-west-2a"),
				newNodeClass("default", "us-west-2a"),
			},
			nodeClassName: "missing",
			wantErr:       true,
		},
		{
			name: "NodeClass name errors when the NodeClass has no subnet zones",
			objs: []client.Object{
				newNodeClass("default"),
			},
			nodeClassName: "default",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := newFakeClient(t, tt.objs...)
			got, err := GetClusterZones(context.Background(), k8sClient, tt.nodeClassName)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
