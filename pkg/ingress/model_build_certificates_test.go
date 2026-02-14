package ingress

import (
	"testing"

	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	acmModel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/acm"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

func Test_buildACMCertificates(t *testing.T) {
	type fields struct {
		ingGroup     Group
		defaultCAArn string
	}

	tests := []struct {
		name   string
		fields fields

		wantErr      bool
		wantCertSpec acmModel.CertificateSpec
		wantNilCert  bool
	}{
		{
			name: "Build certificate for catch-all ingress",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
									},
								},
							},
						},
					},
				},
			},
			wantNilCert: true,
		},
		{
			name: "Build certificate for regular ingress",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
									},
								},
							},
						},
					},
				},
			},
			wantCertSpec: acmModel.CertificateSpec{Type: acmtypes.CertificateTypeAmazonIssued, DomainName: "example.com", SubjectAlternativeNames: []string{"example.com"}, ValidationMethod: acmtypes.ValidationMethodDns, Tags: map[string]string{}},
		},
		{
			name: "Build certificate for multi-host ingress",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
										{Host: "otherexample.com"},
										{Host: "yetanotherexample.com"},
									},
								},
							},
						},
					},
				},
			},
			wantCertSpec: acmModel.CertificateSpec{Type: acmtypes.CertificateTypeAmazonIssued, DomainName: "example.com", SubjectAlternativeNames: []string{"example.com", "otherexample.com", "yetanotherexample.com"}, ValidationMethod: acmtypes.ValidationMethodDns, Tags: map[string]string{}},
		},
		{
			name: "Build certificate for certificate-arn pinned ingress",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
										"alb.ingress.kubernetes.io/certificate-arn": `arn:aws:iam::123456789:server-certificate/new-clb-cert`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
									},
								},
							},
						},
					},
				},
			},
			wantNilCert: true,
		},
		{
			name: "Build certificate for controller-flag PCA ARN ingress",
			fields: fields{
				defaultCAArn: "arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/ee8e7862-1c41-4722-87cb-9ae8e56e8d00",
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-3",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
									},
								},
							},
						},
					},
				},
			},
			wantCertSpec: acmModel.CertificateSpec{Type: acmtypes.CertificateTypePrivate, CertificateAuthorityARN: "arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/ee8e7862-1c41-4722-87cb-9ae8e56e8d00", DomainName: "example.com", SubjectAlternativeNames: []string{"example.com"}, ValidationMethod: acmtypes.ValidationMethodDns, Tags: map[string]string{}},
		},
		{
			name: "Build certificate for PCA ARN annotation ingress",
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
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
										"alb.ingress.kubernetes.io/acm-pca-arn":     `arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/ee8e7862-1c41-4722-87cb-9ae8e56e8d00`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
									},
								},
							},
						},
					},
				},
			},
			wantCertSpec: acmModel.CertificateSpec{Type: acmtypes.CertificateTypePrivate, CertificateAuthorityARN: "arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/ee8e7862-1c41-4722-87cb-9ae8e56e8d00", DomainName: "example.com", SubjectAlternativeNames: []string{"example.com"}, ValidationMethod: acmtypes.ValidationMethodDns, Tags: map[string]string{}},
		},
		{
			name: "Build certificate for PCA ARN override annotation ingress",
			fields: fields{
				defaultCAArn: "arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/ee8e7862-1c41-4722-87cb-9ae8e56e8d00",
				ingGroup: Group{
					ID: GroupID{Name: "explicit-group"},
					Members: []ClassifiedIngress{
						{
							Ing: &networking.Ingress{
								ObjectMeta: metav1.ObjectMeta{
									Namespace: "awesome-ns",
									Name:      "ing-3",
									Annotations: map[string]string{
										"alb.ingress.kubernetes.io/listen-ports":    `[{"HTTP": 80}, {"HTTPS": 443}]`,
										"alb.ingress.kubernetes.io/create-acm-cert": `true`,
										"alb.ingress.kubernetes.io/acm-pca-arn":     `arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/bb4c0627-3ff4-439e-abb1-e5cc03426cc3`,
									},
								},
								Spec: networking.IngressSpec{
									Rules: []networking.IngressRule{
										{Host: "example.com"},
									},
								},
							},
						},
					},
				},
			},
			wantCertSpec: acmModel.CertificateSpec{Type: acmtypes.CertificateTypePrivate, CertificateAuthorityARN: "arn:aws:acm-pca:eu-central-1:134051052098:certificate-authority/bb4c0627-3ff4-439e-abb1-e5cc03426cc3", DomainName: "example.com", SubjectAlternativeNames: []string{"example.com"}, ValidationMethod: acmtypes.ValidationMethodDns, Tags: map[string]string{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				defaultCAArn:     tt.fields.defaultCAArn,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
				stack:            core.NewDefaultStack(core.StackID(tt.fields.ingGroup.ID)),
			}
			got, err := task.buildACMCertificates(t.Context(), &tt.fields.ingGroup.Members[0])
			if tt.wantErr {
				assert.Error(t, err)
				// assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				if tt.wantNilCert {
					var nilCert *acmModel.Certificate
					assert.Equal(t, nilCert, got)
				} else {
					assert.Equal(t, tt.wantCertSpec, got.Spec)
				}
			}
		})
	}
}
