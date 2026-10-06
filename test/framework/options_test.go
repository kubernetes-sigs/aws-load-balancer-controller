package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions_validateLogDelivery(t *testing.T) {
	valid := Options{EnableLogDeliveryTests: true, LogDeliveryLogGroupARN: "arn:aws:logs:us-west-2:111122223333:log-group:/aws/vendedlogs/e2e", LogDeliveryS3BucketARN: "arn:aws:s3:::e2e-logs/prefix"}
	tests := []struct {
		name    string
		options Options
		wantErr string
	}{
		{name: "disabled requires no destinations", options: Options{}},
		{name: "valid destinations", options: valid},
		{name: "enabled requires a log group", options: Options{EnableLogDeliveryTests: true}, wantErr: "--log-delivery-log-group-arn"},
		{name: "enabled requires an S3 bucket", options: Options{EnableLogDeliveryTests: true, LogDeliveryLogGroupARN: valid.LogDeliveryLogGroupARN}, wantErr: "--log-delivery-s3-bucket-arn"},
		{name: "wrong log group destination type", options: Options{EnableLogDeliveryTests: true, LogDeliveryLogGroupARN: valid.LogDeliveryS3BucketARN, LogDeliveryS3BucketARN: valid.LogDeliveryS3BucketARN}, wantErr: "--log-delivery-log-group-arn"},
		{name: "access point is not a bucket", options: Options{EnableLogDeliveryTests: true, LogDeliveryLogGroupARN: valid.LogDeliveryLogGroupARN, LogDeliveryS3BucketARN: "arn:aws:s3:us-west-2:111122223333:accesspoint/e2e"}, wantErr: "--log-delivery-s3-bucket-arn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.options.validateLogDelivery()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
