package config

import (
	"testing"

	"github.com/coreos/alb-ingress-controller/pkg/util/log"
)

func TestParseAnnotations(t *testing.T) {
	_, err := ParseAnnotations(nil)
	if err == nil {
		t.Fatalf("ParseAnnotations should not accept nil for annotations")
	}
}

func TestSetPorts(t *testing.T) {
	var tests = []struct {
		port     string
		certArn  string
		expected []ListenerPort
	}{
		{"", "", []ListenerPort{{Port: int64(80)}}},
		{"", "arn", []ListenerPort{{Port: int64(443), HTTPS: true}}},
		{`[{"HTTP":80}]`, "", []ListenerPort{{Port: int64(80)}}},
		{`[{"HTTPS":8888}]`, "arn", []ListenerPort{{Port: int64(8888), HTTPS: true}}},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setPorts(map[string]string{portKey: tt.port, certificateArnKey: tt.certArn})
		if err != nil {
			t.Errorf("setPorts(%v, %v): errored: %v", tt.port, tt.certArn, err)
			continue
		}
		if log.Prettify(a.Ports) != log.Prettify(tt.expected) {
			t.Errorf("setPorts(%v, %v): expected %v, actual %v", tt.port, tt.certArn, log.Prettify(tt.expected), log.Prettify(a.Ports))
		}
	}
}

func TestSetScheme(t *testing.T) {
	var tests = []struct {
		scheme   string
		expected string
		pass     bool
	}{
		{"", "", false},
		{"internal", "internal", true},
		{"internal", "internet-facing", false},
		{"internet-facing", "internal", false},
		{"internet-facing", "internet-facing", true},
	}

	for _, tt := range tests {
		a := &Annotations{}

		err := a.setScheme(map[string]string{schemeKey: tt.scheme})
		if err != nil && tt.pass {
			t.Errorf("setScheme(%v): expected %v, errored: %v", tt.scheme, tt.expected, err)
		}
		if err == nil && tt.pass && tt.expected != *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
		if err == nil && !tt.pass && tt.expected == *a.Scheme {
			t.Errorf("setScheme(%v): expected %v, actual %v", tt.scheme, tt.expected, *a.Scheme)
		}
	}
}

// TODO: Fix this up, can't compare the pointers
// func TestParseSecurityGroups(t *testing.T) {
// 	setupEC2()
// 	ec2responses["DescribeSecurityGroups"] = &ec2.DescribeSecurityGroupsOutput{
// 		SecurityGroups: []*ec2.SecurityGroup{
// 			&ec2.SecurityGroup{GroupId: aws.String("sg-bcdefg")},
// 		},
// 	}

// 	var tests = []struct {
// 		annotation string
// 		expected   []*string
// 	}{
// 		{"sg-abcdef,test", []*string{aws.String("sg-abcdef")}},
// 	}

// 	for _, tt := range tests {
// 		resp := parseSecurityGroups(mockEC2, tt.annotation)
// 		if !reflect.DeepEqual(tt.expected, resp) {
// 			t.Errorf("parseSecurityGroups(EC2, %v) expected %+v, actual %#v", tt.annotation, tt.expected, resp)
// 		}
// 	}
// }
