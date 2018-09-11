package ls

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"testing"
)

func Test_domainMatchesIngressTLSHost(t *testing.T) {
	var tests = []struct {
		domain string
		host   string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "exampl0.com", false},

		// wildcards
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.exampl0.com", "foo.example.com", false},

		// invalid hosts, not sure these are possible
		{"*.*.example.com", "foo.bar.example.com", false},
		{"foo.*.example.com", "foo.bar.example.com", false},
	}

	for _, test := range tests {
		var msg = "should"
		if !test.want {
			msg = "should not"
		}

		t.Run(fmt.Sprintf("%s %s match %s", test.domain, msg, test.host), func(t *testing.T) {
			have := domainMatchesIngressTLSHost(aws.String(test.domain), aws.String(test.host))
			if test.want != have {
				t.Fail()
			}
		})
	}
}
