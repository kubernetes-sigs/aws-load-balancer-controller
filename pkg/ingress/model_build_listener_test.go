package ingress

import (
	"context"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_computeIngressListenPortConfigByPort(t *testing.T) {
	type fields struct {
		ingGroup Group
	}

	tests := []struct {
		name           string
		fields         fields
		want           map[int64]listenPortConfig
		wantErr        error
		mutualAuthMode string
	}{
		{
			mutualAuthMode: "off",
			name:           "Listener Spec when MutualAuthentication annotation is specified",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":          `[{"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/mutual-authentication": `[{"port":443,"mode":"off"}]`,
										"alb.ingress.kubernetes.io/certificate-arn":       "arn:aws:iam::123456789:server-certificate/new-clb-cert",
									},
								},
							},
						},
					},
				},
			},
		},

		{
			mutualAuthMode: "",
			name:           "Listener Spec when MutualAuthentication annotation is not specified",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-1",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:iam::123456789:server-certificate/new-clb-cert",
									},
								},
							},
						},
					},
				},
			},
			want: map[int64]listenPortConfig{443: listenPortConfig{protocol: "HTTPS", inboundCIDRv4s: []string(nil), inboundCIDRv6s: []string(nil), prefixLists: []string(nil), sslPolicy: (*string)(nil), tlsCerts: []string{"arn:aws:iam::123456789:server-certificate/new-clb-cert"}, mutualAuthentication: nil}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.computeIngressListenPortConfigByPort(context.Background(), &tt.fields.ingGroup.Members[0])
			if err != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {

				if tt.mutualAuthMode != "" {
					assert.Equal(t, tt.mutualAuthMode, got[443].mutualAuthentication.Mode)
				} else {
					assert.Equal(t, tt.want, got)
				}

			}
		})
	}
}
