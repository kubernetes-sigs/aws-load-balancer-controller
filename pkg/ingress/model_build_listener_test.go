package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/aws"
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
		{
			name: "Listener Config when MutualAuthentication annotation is specified with advertise trust store CA not set",
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
			name: "Listener Config when MutualAuthentication annotation is specified with advertise trust store CA set",
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
										"alb.ingress.kubernetes.io/mutual-authentication": `[{"port":443,"mode":"off"}, {"port":80,"mode":"verify", "advertiseTrustStoreCaNames": "on", "trustStore": "arn:aws:elasticloadbalancing:trustStoreArn"}]`,
										"alb.ingress.kubernetes.io/certificate-arn":       "arn:aws:iam::123456789:server-certificate/new-clb-cert",
									},
								},
							},
						},
					},
				},
			},
			want: []WantStruct{{port: 443, mutualAuth: &(elbv2.MutualAuthenticationAttributes{Mode: "off", TrustStoreArn: nil, IgnoreClientCertificateExpiry: nil})}, {port: 80, mutualAuth: &(elbv2.MutualAuthenticationAttributes{Mode: "verify", TrustStoreArn: awssdk.String("arn:aws:elasticloadbalancing:trustStoreArn"), AdvertiseTrustStoreCaNames: awssdk.String("on"), IgnoreClientCertificateExpiry: nil})}},
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

						if mutualAuth.AdvertiseTrustStoreCaNames != nil {
							assert.Equal(t, *mutualAuth.AdvertiseTrustStoreCaNames, *got[port].mutualAuthentication.AdvertiseTrustStoreCaNames)
						}

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

func Test_validateMutualAuthenticationConfig(t *testing.T) {
	tests := []struct {
		name                 string
		port                 int32
		mode                 string
		trustStoreARN        string
		ignoreClientCert     *bool
		advertiseCANames     *string
		expectedErrorMessage *string
	}{
		{
			name: "happy path no validation error off mode",
			port: 800,
			mode: string(elbv2model.MutualAuthenticationOffMode),
		},
		{
			name: "happy path no validation error pass through mode",
			port: 800,
			mode: string(elbv2model.MutualAuthenticationPassthroughMode),
		},
		{
			name:          "happy path no validation error verify mode",
			port:          800,
			mode:          string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN: "truststore",
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry",
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: aws.Bool(true),
		},
		{
			name:             "happy path no validation error verify mode, with ignore client cert expiry false",
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			ignoreClientCert: aws.Bool(false),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca on",
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: aws.String("on"),
		},
		{
			name:             "happy path no validation error verify mode, with advertise ca off",
			port:             800,
			mode:             string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:    "truststore",
			advertiseCANames: aws.String("off"),
		},
		{
			name:                 "no mode",
			port:                 800,
			expectedErrorMessage: awssdk.String("mutualAuthentication mode cannot be empty for port 800"),
		},
		{
			name:                 "unknown mode",
			port:                 800,
			mode:                 "foo",
			expectedErrorMessage: awssdk.String("mutualAuthentication mode value must be among"),
		},
		{
			name:                 "port invalid",
			port:                 800000,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			expectedErrorMessage: awssdk.String("listen port must be within [1, 65535]: 800000"),
		},
		{
			name:                 "missing truststore arn for verify",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			expectedErrorMessage: awssdk.String("trustStore is required when mutualAuthentication mode is verify for port 800"),
		},
		{
			name:                 "truststore arn set but mode not verify",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			trustStoreARN:        "truststore",
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support trustStore for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for off mode",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode off does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "ignore client cert expiry set for passthrough mode",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			ignoreClientCert:     awssdk.Bool(true),
			expectedErrorMessage: awssdk.String("Mutual Authentication mode passthrough does not support ignoring client certificate expiry for port 800"),
		},
		{
			name:                 "advertise ca set for off mode",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationOffMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode off does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set for passthrough mode",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationPassthroughMode),
			advertiseCANames:     awssdk.String("on"),
			expectedErrorMessage: awssdk.String("Authentication mode passthrough does not support advertiseTrustStoreCaNames for port 800"),
		},
		{
			name:                 "advertise ca set with invalid value",
			port:                 800,
			mode:                 string(elbv2model.MutualAuthenticationVerifyMode),
			trustStoreARN:        "truststore",
			advertiseCANames:     awssdk.String("foo"),
			expectedErrorMessage: awssdk.String("advertiseTrustStoreCaNames only supports the values \"on\" and \"off\" got value foo for port 800"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			res := task.validateMutualAuthenticationConfig(tt.port, tt.mode, tt.trustStoreARN, tt.ignoreClientCert, tt.advertiseCANames)

			if tt.expectedErrorMessage == nil {
				assert.Nil(t, res)
			} else {
				assert.Contains(t, res.Error(), *tt.expectedErrorMessage)
			}
		})
	}
}
