package ingress

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/pkg/model/elbv2"
)

func Test_computeIngressListenPortConfigByPort_MutualAuthentication(t *testing.T) {
	type fields struct {
		ingGroup Group
	}
	type WantStruct struct {
		port       int32
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
func Test_buildListenerAttributes(t *testing.T) {
	type fields struct {
		ingGroup Group
	}

	tests := []struct {
		name   string
		fields fields

		wantErr   bool
		wantValue []elbv2model.ListenerAttribute
	}{
		{
			name: "Listener attribute annotation value is not stringMap",
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
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "attrKey",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Listener attribute annotation is not specified",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-2",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports": `[{"HTTP": 80}]`,
									},
								},
							},
						},
					},
				},
			},
			wantErr:   false,
			wantValue: []elbv2model.ListenerAttribute{},
		},
		{
			name: "Listener attribute annotation is specified",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-3",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "routing.http.response.server.enabled=false",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantValue: []elbv2model.ListenerAttribute{
				{
					Key:   "routing.http.response.server.enabled",
					Value: "false",
				},
			},
		},
		{
			name: "Listener attribute conflict",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-4",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "routing.http.response.server.enabled=false",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-5",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "routing.http.response.server.enabled=true",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "merge Listener attributes",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-4",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "attrKey1=attrValue1",
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-5",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "attrKey2=attrValue2",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantValue: []elbv2model.ListenerAttribute{
				{
					Key:   "attrKey1",
					Value: "attrValue1",
				},
				{
					Key:   "attrKey2",
					Value: "attrValue2",
				},
			},
		},
		{
			name: "Ignore conflicting value when the key is specified by ingress class param",
			fields: fields{
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-6",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "attrKey1=attrValue1",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &elbv2api.IngressClassParams{
									ObjectMeta: metav1.ObjectMeta{
										Name: "awesome-class",
									},
									Spec: elbv2api.IngressClassParamsSpec{
										Listeners: []elbv2api.Listener{
											{
												Protocol: "HTTP",
												Port:     80,
												ListenerAttributes: []elbv2api.Attribute{
													{
														Key:   "attrKey1",
														Value: "attrValue1",
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-7",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":                `[{"HTTP": 80}]`,
										"alb.ingress.kubernetes.io/listener-attributes.HTTP-80": "attrKey1=attrValue2",
									},
								},
							},
							IngClassConfig: ClassConfiguration{
								IngClassParams: &elbv2api.IngressClassParams{
									ObjectMeta: metav1.ObjectMeta{
										Name: "awesome-class",
									},
									Spec: elbv2api.IngressClassParamsSpec{
										Listeners: []elbv2api.Listener{
											{
												Protocol: "HTTP",
												Port:     80,
												ListenerAttributes: []elbv2api.Attribute{
													{
														Key:   "attrKey1",
														Value: "attrValue1",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
			wantValue: []elbv2model.ListenerAttribute{
				{
					Key:   "attrKey1",
					Value: "attrValue1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}

			listenerAttributes, err := task.buildListenerAttributes(context.Background(), tt.fields.ingGroup.Members, 80, "HTTP")
			t.Log(listenerAttributes)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.ElementsMatch(t, tt.wantValue, listenerAttributes)
			}

		})
	}
}
