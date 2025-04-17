package routeutils

import (
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_doesRouteAttachToGateway(t *testing.T) {
	testCases := []struct {
		name   string
		gw     gwv1.Gateway
		route  preLoadRouteDescriptor
		result bool
	}{
		{
			name: "parent ref has nil kind and matching name / namespace",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "gw",
							},
						},
					},
				},
			}),
			result: true,
		},
		{
			name: "parent ref has gateway kind and matching name / namespace default to gw namespace when ref doesnt have one",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "gw",
								Kind: (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
			result: true,
		},
		{
			name: "parent ref has gateway kind and matching name / namespace default to gw namespace",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gw",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
			result: true,
		},
		{
			name: "multiple parent refs should return true when one matches",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gw2",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Other")),
							},
							{
								Name:      "gw2",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
							{
								Name:      "gw3",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
							{
								Name:      "gw",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
			result: true,
		},
		{
			name: "multiple parent refs should return false if none matches",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gw2",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Other")),
							},
							{
								Name:      "gw2",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
							{
								Name:      "gw3",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
							{
								Name:      "gw4",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns1")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
		},
		{
			name: "parent ref has gateway kind and matching name but namespace different",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gw",
								Namespace: (*gwv1.Namespace)(awssdk.String("ns2")),
								Kind:      (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
		},
		{
			name: "parent ref has gateway kind and matching name but name different",
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "ns1",
				},
			},
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "gw-other",
								Kind: (*gwv1.Kind)(awssdk.String("Gateway")),
							},
						},
					},
				},
			}),
		},
		{
			name:  "no parent refs",
			route: convertHTTPRoute(gwv1.HTTPRoute{}),
		},
		{
			name: "parent ref has non gateway kind",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Kind: (*gwv1.Kind)(awssdk.String("other kind")),
							},
						},
					},
				},
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := &routeAttachmentHelperImpl{
				logger: logr.Discard(),
			}
			assert.Equal(t, tc.result, helper.doesRouteAttachToGateway(tc.gw, tc.route))
		})
	}
}

func Test_routeAllowsAttachmentToListener(t *testing.T) {
	testCases := []struct {
		name     string
		listener gwv1.Listener
		route    preLoadRouteDescriptor
		result   bool
	}{
		{
			name: "allows attachment section and port correct",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
						},
					},
				},
			}),
			listener: gwv1.Listener{
				Name: "sectionname",
				Port: 80,
			},
			result: true,
		},
		{
			name: "allows attachment section specified",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname")),
							},
						},
					},
				},
			}),
			listener: gwv1.Listener{
				Name: "sectionname",
				Port: 80,
			},
			result: true,
		},
		{
			name: "allows attachment port specified",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Port: (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
						},
					},
				},
			}),
			listener: gwv1.Listener{
				Name: "sectionname",
				Port: 80,
			},
			result: true,
		},
		{
			name: "multiple parent refs one ref allows attachment",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname1")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname2")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname3")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
						},
					},
				},
			}),
			listener: gwv1.Listener{
				Name: "sectionname",
				Port: 80,
			},
			result: true,
		},
		{
			name: "multiple parent refs one ref none attachment",
			route: convertHTTPRoute(gwv1.HTTPRoute{
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname1")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname2")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname3")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
							{
								SectionName: (*gwv1.SectionName)(awssdk.String("sectionname4")),
								Port:        (*gwv1.PortNumber)(awssdk.Int32(80)),
							},
						},
					},
				},
			}),
			listener: gwv1.Listener{
				Name: "sectionname",
				Port: 80,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			helper := &routeAttachmentHelperImpl{
				logger: logr.Discard(),
			}
			assert.Equal(t, tc.result, helper.routeAllowsAttachmentToListener(tc.listener, tc.route))
		})
	}
}
