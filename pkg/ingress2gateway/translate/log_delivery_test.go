package translate

import (
	"testing"

	"github.com/stretchr/testify/require"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/v3/apis/elbv2/v1beta1"
	gatewayapi "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/ingress2gateway"
)

func TestTranslateIngressClassLogDelivery(t *testing.T) {
	deliveries := []elbv2api.LogDeliveryConfiguration{{
		LogType:        "ALB_ACCESS_LOGS",
		DestinationARN: ptr.To("arn:aws:s3:::class-logs"),
		OutputFormat:   ptr.To("plain"),
		FieldDelimiter: ptr.To(","),
		S3DeliveryConfiguration: &elbv2api.LogDeliveryS3Configuration{
			SuffixPath:               ptr.To("{yyyy}/{MM}/{dd}"),
			EnableHiveCompatiblePath: ptr.To(true),
		},
	}, {
		LogType:                "ALB_CONNECTION_LOGS",
		DeliveryDestinationARN: ptr.To("arn:aws:logs:us-west-2:111122223333:delivery-destination:shared"),
	}}
	want := []gatewayapi.LogDeliveryConfiguration{{
		LogType:        "ALB_ACCESS_LOGS",
		DestinationArn: ptr.To("arn:aws:s3:::class-logs"),
		OutputFormat:   ptr.To("plain"),
		FieldDelimiter: ptr.To(","),
		S3DeliveryConfiguration: &gatewayapi.LogDeliveryS3Configuration{
			SuffixPath:               ptr.To("{yyyy}/{MM}/{dd}"),
			EnableHiveCompatiblePath: ptr.To(true),
		},
	}, {
		LogType:                "ALB_CONNECTION_LOGS",
		DeliveryDestinationArn: ptr.To("arn:aws:logs:us-west-2:111122223333:delivery-destination:shared"),
	}}
	for _, tt := range []struct {
		name        string
		first       *[]elbv2api.LogDeliveryConfiguration
		second      *[]elbv2api.LogDeliveryConfiguration
		annotations []string
		want        []gatewayapi.LogDeliveryConfiguration
		wantErr     string
	}{
		{
			name:  "class configuration overrides conflicting and malformed group annotations",
			first: &deliveries, annotations: []string{"not-json", "[]"}, want: want,
		},
		{
			name:  "class configuration creates LB config without annotations",
			first: &deliveries, annotations: []string{""}, want: want,
		},
		{
			name:        "empty class configuration disables annotated delivery",
			first:       ptr.To([]elbv2api.LogDeliveryConfiguration{}),
			annotations: []string{`[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::annotation-logs"}]`, "not-json"},
			want:        []gatewayapi.LogDeliveryConfiguration{},
		},
		{
			name:  "matching class configurations",
			first: &deliveries, second: &deliveries, annotations: []string{"not-json", "[]"}, want: want,
		},
		{
			name:  "conflicting class configurations are rejected",
			first: &deliveries, second: ptr.To([]elbv2api.LogDeliveryConfiguration{}), annotations: []string{"", ""},
			wantErr: "conflicting IngressClassParams log-delivery",
		},
		{
			name:        "annotation fallback without class configuration",
			annotations: []string{`[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::annotation-logs"}]`},
			want:        []gatewayapi.LogDeliveryConfiguration{{LogType: "ALB_ACCESS_LOGS", DestinationArn: ptr.To("arn:aws:s3:::annotation-logs")}},
		},
		{
			name:        "annotation conflicts are still rejected without class configuration",
			annotations: []string{`[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::annotation-logs"}]`, "[]"},
			wantErr:     "conflicting annotation alb.ingress.kubernetes.io/log-delivery",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in := &ingress2gateway.InputResources{}
			for i, value := range tt.annotations {
				name := []string{"first", "second"}[i]
				annos := map[string]string{"alb.ingress.kubernetes.io/group.name": "shared"}
				if value != "" {
					annos["alb.ingress.kubernetes.io/log-delivery"] = value
				}
				in.Ingresses = append(in.Ingresses, networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "apps", Annotations: annos},
					Spec:       networking.IngressSpec{IngressClassName: &name},
				})
				in.IngressClasses = append(in.IngressClasses, networking.IngressClass{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Spec: networking.IngressClassSpec{
						Controller: "ingress.k8s.aws/alb",
						Parameters: &networking.IngressClassParametersReference{APIGroup: ptr.To("elbv2.k8s.aws"), Kind: "IngressClassParams", Name: name},
					},
				})
				in.IngressClassParams = append(in.IngressClassParams, elbv2api.IngressClassParams{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Spec:       elbv2api.IngressClassParamsSpec{LogDelivery: []*[]elbv2api.LogDeliveryConfiguration{tt.first, tt.second}[i]},
				})
			}
			out, err := Translate(in)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, out.LoadBalancerConfigurations, 1)
			require.Equal(t, tt.want, out.LoadBalancerConfigurations[0].Spec.LogDelivery)
		})
	}
}
