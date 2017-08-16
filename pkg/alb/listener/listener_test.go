package listener

import (
	"testing"
	//"github.com/aws/aws-sdk-go/aws"
	//"github.com/aws/aws-sdk-go/aws/awserr"
	//"github.com/aws/aws-sdk-go/aws/awsutil"
	//"github.com/aws/aws-sdk-go/service/elbv2"
)

/*func TestNewListener(t *testing.T) {
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
		listener := NewListener(tt.annotations, aws.String("ingressID")).DesiredListener
		l := &Listener{
			CurrentListener: listener,
		}
		if l.needsModification(tt.listener) && tt.pass {
			t.Errorf("NewListener(%v) returned an unexpected listener:\n%s\n!=\n%s", awsutil.Prettify(tt.annotations), awsutil.Prettify(listener), awsutil.Prettify(tt.listener))
		}
	}
}*/

/*func TestListenerCreate(t *testing.T) {
	setup()

	var tests = []struct {
		DesiredListener *elbv2.Listener
		Output          *elbv2.Listener
		Error           awserr.Error
		pass            bool
	}{
		{
			NewListener(&annotationsT{}, aws.String("ingressID")).DesiredListener,
			&elbv2.Listener{
				DefaultActions: []*elbv2.Action{
					&elbv2.Action{
						TargetGroupArn: aws.String("TargetGroupArn"),
						Type:           aws.String("forward"),
					},
				},
				ListenerArn:     aws.String("some:arn"),
				LoadBalancerArn: aws.String("arn"),
				Port:            aws.Int64(80),
				Protocol:        aws.String("HTTP"),
			},
			nil,
			true,
		},
		{
			NewListener(&annotationsT{}, aws.String("ingressID")).DesiredListener,
			nil,
			awserr.New("TargetGroupAssociationLimit", "", nil),
			false,
		},
		{
			NewListener(&annotationsT{}, aws.String("ingressID")).DesiredListener,
			nil,
			awserr.New("Some other error", "", nil),
			false,
		},
	}

	lb := &LoadBalancer{
<<<<<<< HEAD:controller/alb/listener_test.go
		Id:                  aws.String("test-alb"),
=======
		id:                  aws.String("test-alb"),
		ingressId:           aws.String("ingressId"),
>>>>>>> master:pkg/cmd/controller/elbv2_listener_test.go
		CurrentLoadBalancer: &elbv2.LoadBalancer{LoadBalancerArn: aws.String("arn")},
	}

	tg := &TargetGroup{
		CurrentTargetGroup: &elbv2.TargetGroup{
			TargetGroupArn: aws.String("TargetGroupArn"),
		},
	}

	for n, tt := range tests {
		mockedELBV2responses.Error = tt.Error

		l := &Listener{
			IngressId:       aws.String("ingressID"),
			DesiredListener: tt.DesiredListener,
		}

		err := l.create(lb, tg)
		if err != nil && tt.pass {
			t.Errorf("%d: listener.create() returned an error: %v", n, err)
		}
		if err == nil && !tt.pass {
			t.Errorf("%d: listener.create() did not error but should have", n)
		}

		if l.needsModification(tt.Output) && tt.pass {
			t.Errorf("%d: listener.create() did not create what was expected, %v\n  !=\n%v", n, l.CurrentListener, tt.Output)
		}
	}
}*/

func TestListenerModify(t *testing.T) {

}

/*func TestListenerDelete(t *testing.T) {
	setup()

	var tests = []struct {
		CurrentListener *elbv2.Listener
		Error           awserr.Error
		pass            bool
	}{
		{
			&elbv2.Listener{ListenerArn: aws.String("some:arn")},
			nil,
			true,
		},
		{
			&elbv2.Listener{ListenerArn: aws.String("some:arn")},
			awserr.New("Some error happened", "", nil),
			false,
		},
	}

	for n, tt := range tests {
		mockedELBV2responses.Error = tt.Error
		l := &Listener{
			IngressId:       aws.String("IngressId"),
			CurrentListener: tt.CurrentListener,
		}

		err := l.delete(&LoadBalancer{ingressId: aws.String("ingressID")})
		if err != nil && tt.pass {
			t.Errorf("%d: listener.delete() returned an error: %v", n, err)
		}
		if err == nil && !tt.pass {
			t.Errorf("%d: listener.delete() did not error but should have", n)
		}
	}
}*/

/*func TestListenerEquals(t *testing.T) {
func TestListenerNeedsModification(t *testing.T) {
	setup()

	var tests = []struct {
		CurrentListener *elbv2.Listener
		TargetListener  *elbv2.Listener
		equals          bool
	}{
		{ // Port does not need modification
			&elbv2.Listener{Port: aws.Int64(123)},
			&elbv2.Listener{Port: aws.Int64(123)},
			false,
		},
		{ // Port needs modification
			&elbv2.Listener{Port: aws.Int64(123)},
			&elbv2.Listener{Port: aws.Int64(1234)},
			true,
		},
		{ // Protocol does not need modification
			&elbv2.Listener{Protocol: aws.String("HTTP")},
			&elbv2.Listener{Protocol: aws.String("HTTP")},
			false,
		},
		{ // Protocol needs modification
			&elbv2.Listener{Protocol: aws.String("HTTP")},
			&elbv2.Listener{Protocol: aws.String("HTTPS")},
			true,
		},
		{ // Certificates does not need modification
			&elbv2.Listener{Certificates: []*elbv2.Certificate{&elbv2.Certificate{CertificateArn: aws.String("arn")}}},
			&elbv2.Listener{Certificates: []*elbv2.Certificate{&elbv2.Certificate{CertificateArn: aws.String("arn")}}},
			false,
		},
		{ // Protocol needs modification
			&elbv2.Listener{Certificates: []*elbv2.Certificate{&elbv2.Certificate{CertificateArn: aws.String("arn")}}},
			&elbv2.Listener{Certificates: []*elbv2.Certificate{&elbv2.Certificate{CertificateArn: aws.String("arn_")}}},
			true,
		},
	}

	for n, tt := range tests {
		mockedELBV2responses.Error = nil
		l := &Listener{
			CurrentListener: tt.CurrentListener,
		}

		equals := l.needsModification(tt.TargetListener)
		if equals != tt.equals {
			t.Errorf("%d: listener.needsModification() returned %v, should have returned %v", n, equals, tt.equals)
		}
	}
}*/
