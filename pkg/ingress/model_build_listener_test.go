package ingress

import (
	"context"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	"testing"
)

func Test_computeIngressListenPortConfigByPort_MutualAuthentication(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type WantStruct struct {
		port       int64
		mutualAuth *elbv2.MutualAuthenticationAttributes
	}

	tests := []struct {
		name   string
		fields fields

		wantErr        error
		mutualAuthMode string
		want           []WantStruct
	}{
		{
			name: "Listener Config when MutualAuthentication annotation is specified",
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
										"alb.ingress.kubernetes.io/listen-ports":          `[{"HTTPS": 443}, {"HTTPS": 80}]`,
										"alb.ingress.kubernetes.io/mutual-authentication": `[{"port":443,"mode":"off"}, {"port":80,"mode":"passthrough"}]`,
										"alb.ingress.kubernetes.io/certificate-arn":       "arn:aws:iam::123456789:server-certificate/new-clb-cert",
									},
								},
							},
						},
					},
				},
			},
			want: []WantStruct{{port: 443, mutualAuth: &(elbv2.MutualAuthenticationAttributes{Mode: "off", TrustStoreArn: nil, IgnoreClientCertificateExpiry: nil})}, {port: 80, mutualAuth: &(elbv2.MutualAuthenticationAttributes{Mode: "passthrough", TrustStoreArn: nil, IgnoreClientCertificateExpiry: nil})}},
		},

		{

			name: "Listener Config when MutualAuthentication annotation is not specified",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTPS": 443}, {"HTTPS": 80}]`,
										"alb.ingress.kubernetes.io/certificate-arn": "arn:aws:iam::123456789:server-certificate/new-clb-cert",
									},
								},
							},
						},
					},
				},
			},
			want: []WantStruct{{port: 443, mutualAuth: nil}, {port: 80, mutualAuth: nil}},
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

				for i := 0; i < len(tt.want); i++ {
					port := tt.want[i].port
					mutualAuth := tt.want[i].mutualAuth
					if mutualAuth != nil {
						assert.Equal(t, mutualAuth.Mode, got[port].mutualAuthentication.Mode)
					} else {
						assert.Equal(t, mutualAuth, got[port].mutualAuthentication)
					}

				}

			}
		})
	}
}
