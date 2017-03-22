package controller

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func TestNewListener(t *testing.T) {
	setup()

	var tests = []struct {
		annotations *annotationsT
		listener    *elbv2.Listener
		pass        bool
	}{
		{ // Test defaults
			&annotationsT{},
			&elbv2.Listener{
				Port:     aws.Int64(80),
				Protocol: aws.String("HTTP"),
			},
			true,
		},
		{ // Test port annotations
			&annotationsT{port: aws.Int64(9999)},
			&elbv2.Listener{
				Port:     aws.Int64(9999),
				Protocol: aws.String("HTTP"),
			},
			true,
		},
		{ // Test adding certificateArn annotation sets ARN, port, and protocol
			&annotationsT{
				certificateArn: aws.String("arn:blah"),
			},
			&elbv2.Listener{
				Certificates: []*elbv2.Certificate{
					&elbv2.Certificate{CertificateArn: aws.String("arn:blah")},
				},
				Port:     aws.Int64(443),
				Protocol: aws.String("HTTPS"),
			},
			true,
		},
		{ // Test overriding HTTPS port
			&annotationsT{
				certificateArn: aws.String("arn:blah"),
				port:           aws.Int64(9999),
			},
			&elbv2.Listener{
				Certificates: []*elbv2.Certificate{
					&elbv2.Certificate{CertificateArn: aws.String("arn:blah")},
				},
				Port:     aws.Int64(9999),
				Protocol: aws.String("HTTPS"),
			},
			true,
		},
		{ // Test equals: certificates
			&annotationsT{
				certificateArn: aws.String("arn:blah"),
			},
			&elbv2.Listener{
				Certificates: []*elbv2.Certificate{
					&elbv2.Certificate{CertificateArn: aws.String("arn:bad")},
				},
				Port:     aws.Int64(443),
				Protocol: aws.String("HTTPS"),
			},
			false,
		},
		{ // Test equals: port
			&annotationsT{},
			&elbv2.Listener{
				Port:     aws.Int64(9999),
				Protocol: aws.String("HTTP"),
			},
			false,
		},
		{ // Test equals: protocol
			&annotationsT{},
			&elbv2.Listener{
				Port:     aws.Int64(80),
				Protocol: aws.String("HTTPS"),
			},
			false,
		},
	}

	for _, tt := range tests {
		listener := NewListener(tt.annotations)
		l := &Listener{
			CurrentListener: listener,
		}
		if !l.Equals(tt.listener) && tt.pass {
			t.Errorf("NewListener(%v) returned an unexpected listener:\n%s\n!=\n%s", awsutil.Prettify(tt.annotations), awsutil.Prettify(listener), awsutil.Prettify(tt.listener))
		}
	}
}

func TestListenerCreate(t *testing.T) {
	setup()

	var tests = []struct {
		annotations *annotationsT
		listener    *elbv2.Listener
		pass        bool
	}{}

	for _, tt := range tests {
		listener := NewListener(tt.annotations)
		l := &Listener{
			CurrentListener: listener,
		}
		if !l.Equals(tt.listener) && tt.pass {
			t.Errorf("NewListener(%v) returned an unexpected listener:\n%s\n!=\n%s", awsutil.Prettify(tt.annotations), awsutil.Prettify(listener), awsutil.Prettify(tt.listener))
		}
	}

}
