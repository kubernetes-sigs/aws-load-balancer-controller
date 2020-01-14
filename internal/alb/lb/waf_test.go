package lb

import (
	"context"
	"testing"

	"github.com/kubernetes-sigs/aws-alb-ingress-controller/internal/ingress/annotations/parser"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/mocks"
	extensions "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/cache"
)

func Test_defaultWAFController_getDesiredWebACLId(t *testing.T) {
	tests := []struct {
		name string
		ing  *extensions.Ingress
		want string
	}{
		{
			name: "ingress without waf settings",
			ing: &extensions.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name:        "ingress",
					Annotations: map[string]string{},
				},
			},
			want: "",
		},
		{
			name: "ingress with web-acl-id(waf classic)",
			ing: &extensions.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name: "ingress",
					Annotations: map[string]string{
						parser.AnnotationsPrefix + "/web-acl-id": "my-web-acl-id",
					},
				},
			},
			want: "my-web-acl-id",
		},
		{
			name: "ingress with waf-acl-id(waf classic)",
			ing: &extensions.Ingress{
				ObjectMeta: v1.ObjectMeta{
					Name: "ingress",
					Annotations: map[string]string{
						parser.AnnotationsPrefix + "/waf-acl-id": "my-web-acl-id",
					},
				},
			},
			want: "my-web-acl-id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &defaultWAFController{
				cloud:              &mocks.CloudAPI{},
				webACLIdForLBCache: cache.NewLRUExpireCache(10),
			}
			if got := c.getDesiredWebACLId(context.Background(), tt.ing); got != tt.want {
				t.Errorf("getDesiredWebACLId() = %v, want %v", got, tt.want)
			}
		})
	}
}
