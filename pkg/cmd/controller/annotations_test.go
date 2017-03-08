package controller

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
)

func TestParsePort(t *testing.T) {
	var tests = []struct {
		port     string
		certArn  string
		expected *int64
	}{
		{"", "", aws.Int64(int64(80))},
		{"80", "", aws.Int64(int64(80))},
		{"", "arn", aws.Int64(int64(443))},
		{"80", "arn", aws.Int64(int64(80))},
		{"999", "arn", aws.Int64(int64(999))},
	}

	for _, tt := range tests {
		port := parsePort(tt.port, tt.certArn)
		if *port != *tt.expected {
			t.Errorf("parsePort(%v, %v): expected %v, actual %v", tt.port, tt.certArn, *tt.expected, *port)
		}
	}
}

func TestParseScheme(t *testing.T) {
	var tests = []struct {
		scheme string
		pass   bool
	}{
		{"", false},
		{"/", false},
		{"internal", true},
		{"internet-facing", true},
	}

	for _, tt := range tests {
		_, err := parseScheme(tt.scheme)
		if err != nil && tt.pass {
			t.Errorf("parseScheme(%v): expected %v, actual %v", tt.scheme, tt.pass, err)
		}
		if err == nil && !tt.pass {
			t.Errorf("parseScheme(%v): expected %v, actual %v", tt.scheme, tt.pass, err)
		}
	}

}
