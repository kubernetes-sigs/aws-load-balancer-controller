package v1beta1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIngressClassParamsLogDeliveryRoundTrip(t *testing.T) {
	for _, payload := range []string{
		`{"spec":{}}`,
		`{"spec":{"logDelivery":[]}}`,
		`{"spec":{"logDelivery":[{"logType":"ALB_ACCESS_LOGS","destinationArn":"arn:aws:s3:::class-logs","s3DeliveryConfiguration":{"enableHiveCompatiblePath":true}}]}}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var original IngressClassParams
			require.NoError(t, json.Unmarshal([]byte(payload), &original))
			copied := original.DeepCopy()
			encoded, err := json.Marshal(copied)
			require.NoError(t, err)
			var roundTripped IngressClassParams
			require.NoError(t, json.Unmarshal(encoded, &roundTripped))
			require.Equal(t, original.Spec.LogDelivery, roundTripped.Spec.LogDelivery)
			if original.Spec.LogDelivery != nil && len(*original.Spec.LogDelivery) > 0 {
				*(*copied.Spec.LogDelivery)[0].DestinationARN = "arn:aws:s3:::changed"
				*(*copied.Spec.LogDelivery)[0].S3DeliveryConfiguration.EnableHiveCompatiblePath = false
				require.Equal(t, "arn:aws:s3:::class-logs", *(*original.Spec.LogDelivery)[0].DestinationARN)
				require.True(t, *(*original.Spec.LogDelivery)[0].S3DeliveryConfiguration.EnableHiveCompatiblePath)
			}
		})
	}
}
