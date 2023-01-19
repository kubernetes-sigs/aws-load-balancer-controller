package ingress

import (
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	"testing"
)

func Test_defaultModelBuildTask_classifyRulesByHost(t *testing.T) {
	type args struct {
		rules []networking.IngressRule
	}
	tests := []struct {
		name                        string
		args                        args
		wantRulesWithReplicateHosts []networking.IngressRule
		wantRulesWithUniqueHost     []networking.IngressRule
		wantErr                     error
	}{
		{
			name: "2 rules with no host",
			args: args{
				rules: []networking.IngressRule{
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathA",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathB",
										PathType: (*networking.PathType)(awssdk.String("Exact")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathC",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathD",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantRulesWithReplicateHosts: []networking.IngressRule{
				{
					Host: "",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "/pathA",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-1",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
								{
									Path:     "/pathB",
									PathType: (*networking.PathType)(awssdk.String("Exact")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-2",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "/pathC",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-1",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
								{
									Path:     "/pathD",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-2",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantRulesWithUniqueHost: []networking.IngressRule(nil),
			wantErr:                 nil,
		},
		{
			name: "3 rules with 2 hosts",
			args: args{
				rules: []networking.IngressRule{
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathA",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathB",
										PathType: (*networking.PathType)(awssdk.String("Exact")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathC",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathD",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathE",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantRulesWithReplicateHosts: []networking.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "/pathC",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-1",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
								{
									Path:     "/pathD",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-2",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "/pathE",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-1",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantRulesWithUniqueHost: []networking.IngressRule{
				{
					Host: "",
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Path:     "/pathA",
									PathType: (*networking.PathType)(awssdk.String("Prefix")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-1",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
								{
									Path:     "/pathB",
									PathType: (*networking.PathType)(awssdk.String("Exact")),
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "svc-2",
											Port: networking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			gotRulesWithReplicateHosts, gotRulesWithUniquetHost, err := task.classifyRulesByHost(tt.args.rules)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, gotRulesWithReplicateHosts, tt.wantRulesWithReplicateHosts)
				assert.Equal(t, gotRulesWithUniquetHost, tt.wantRulesWithUniqueHost)
			}
		})
	}
}

func Test_defaultModelBuildTask_getMergeRuleRefMaps(t *testing.T) {
	type args struct {
		rules []networking.IngressRule
	}
	tests := []struct {
		name                 string
		args                 args
		wantMergePathsRefMap map[[5]string][]networking.HTTPIngressPath
		//wantPathToRuleMap    map[networking.HTTPIngressPath]int
	}{
		{
			name: "2 rules with no host, different svc name and same port number",
			args: args{
				rules: []networking.IngressRule{
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathA",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathB",
										PathType: (*networking.PathType)(awssdk.String("Exact")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathC",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathD",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantMergePathsRefMap: map[[5]string][]networking.HTTPIngressPath{
				{"", "Prefix", "svc-1", "", "80"}: {
					{
						Path:     "/pathA",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-1",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
					{
						Path:     "/pathC",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-1",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
				{"", "Prefix", "svc-2", "", "80"}: {
					{
						Path:     "/pathD",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-2",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
				{"", "Exact", "svc-2", "", "80"}: {
					{
						Path:     "/pathB",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-2",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
			},
			//wantPathToRuleMap: map[networking.HTTPIngressPath]int{
			//	{
			//		Path:     "/pathA",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-1",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path:     "/pathB",
			//		PathType: (*networking.PathType)(awssdk.String("Exact")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-2",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path:     "/pathC",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-1",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 1,
			//	{
			//		Path:     "/pathD",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-2",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 1,
			//},
		},
		{
			name: "2 rules with different hosts",
			args: args{
				rules: []networking.IngressRule{
					{
						Host: "",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathA",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathB",
										PathType: (*networking.PathType)(awssdk.String("Exact")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path:     "/pathC",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
									{
										Path:     "/pathD",
										PathType: (*networking.PathType)(awssdk.String("Prefix")),
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Number: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantMergePathsRefMap: map[[5]string][]networking.HTTPIngressPath{
				{"", "Prefix", "svc-1", "", "80"}: {
					{
						Path:     "/pathA",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-1",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
				{"", "Exact", "svc-2", "", "80"}: {
					{
						Path:     "/pathB",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-2",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
				{"example.com", "Prefix", "svc-1", "", "80"}: {
					{
						Path:     "/pathC",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-1",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
				{"example.com", "Prefix", "svc-2", "", "80"}: {
					{
						Path:     "/pathD",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-2",
								Port: networking.ServiceBackendPort{
									Number: 80,
								},
							},
						},
					},
				},
			},
			//wantPathToRuleMap: map[networking.HTTPIngressPath]int{
			//	{
			//		Path:     "/pathA",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-1",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path:     "/pathB",
			//		PathType: (*networking.PathType)(awssdk.String("Exact")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-2",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path:     "/pathC",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-1",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 1,
			//	{
			//		Path:     "/pathD",
			//		PathType: (*networking.PathType)(awssdk.String("Prefix")),
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-2",
			//				Port: networking.ServiceBackendPort{
			//					Number: 80,
			//				},
			//			},
			//		},
			//	}: 1,
			//},
		},
		{
			name: "2 rules with different hosts, different svc name and different port name",
			args: args{
				rules: []networking.IngressRule{
					{
						Host: "example1.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path: "/pathA",
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-1",
												Port: networking.ServiceBackendPort{
													Name: "http",
												},
											},
										},
									},
									{
										Path: "/pathB",
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-2",
												Port: networking.ServiceBackendPort{
													Name: "http",
												},
											},
										},
									},
								},
							},
						},
					},
					{
						Host: "example2.com",
						IngressRuleValue: networking.IngressRuleValue{
							HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{
									{
										Path: "/pathC",
										Backend: networking.IngressBackend{
											Service: &networking.IngressServiceBackend{
												Name: "svc-3",
												Port: networking.ServiceBackendPort{
													Name: "https",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantMergePathsRefMap: map[[5]string][]networking.HTTPIngressPath{
				{"example1.com", "", "svc-1", "http", "0"}: {
					{
						Path: "/pathA",
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-1",
								Port: networking.ServiceBackendPort{
									Name: "http",
								},
							},
						},
					},
				},
				{"example1.com", "", "svc-2", "http", "0"}: {
					{
						Path: "/pathB",
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-2",
								Port: networking.ServiceBackendPort{
									Name: "http",
								},
							},
						},
					},
				},
				{"example2.com", "", "svc-3", "https", "0"}: {
					{
						Path: "/pathC",
						Backend: networking.IngressBackend{
							Service: &networking.IngressServiceBackend{
								Name: "svc-3",
								Port: networking.ServiceBackendPort{
									Name: "https",
								},
							},
						},
					},
				},
			},
			//wantPathToRuleMap: map[networking.HTTPIngressPath]int{
			//	{
			//		Path: "/pathA",
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-1",
			//				Port: networking.ServiceBackendPort{
			//					Name: "http",
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path: "/pathB",
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-2",
			//				Port: networking.ServiceBackendPort{
			//					Name: "http",
			//				},
			//			},
			//		},
			//	}: 0,
			//	{
			//		Path: "/pathC",
			//		Backend: networking.IngressBackend{
			//			Service: &networking.IngressServiceBackend{
			//				Name: "svc-3",
			//				Port: networking.ServiceBackendPort{
			//					Name: "https",
			//				},
			//			},
			//		},
			//	}: 1,
			//},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got := task.getMergeRuleRefMaps(tt.args.rules)
			assert.Equal(t, got, tt.wantMergePathsRefMap)
		})
	}
}

func Test_defaultModelBuildTask_sortIngressPath(t *testing.T) {
	type args struct {
		paths []networking.HTTPIngressPath
	}
	tests := []struct {
		name string
		args args
		want []networking.HTTPIngressPath
	}{
		{
			name: "Exact path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
		},
		{
			name: "Prefix path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/tea",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/tea",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
		},
		{
			name: "ImplementationSpecific path only",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/a",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/test",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/a",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/test",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "Exact and prefix paths",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/abc/abc/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/abc/abc/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa/bbb",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
		},
		{
			name: "Prefix and ImplementationSpecific paths",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/b",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/b",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "All three types",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/b",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/example",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/test",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			want: []networking.HTTPIngressPath{
				{
					Path:     "/test",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
				{
					Path:     "/example",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/b",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, _ := task.sortIngressPaths(tt.args.paths)
			assert.Equal(t, got, tt.want)
		})
	}
}

func Test_defaultModelBuildTask_classifyIngressPathsByType(t *testing.T) {
	type args struct {
		paths []networking.HTTPIngressPath
	}
	tests := []struct {
		name                            string
		args                            args
		wantExactPaths                  []networking.HTTPIngressPath
		wantPrefixPaths                 []networking.HTTPIngressPath
		wantImplementationSpecificPaths []networking.HTTPIngressPath
		wantErr                         error
	}{
		{
			name: "Paths contain path with invalid pathType, return error",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("xyz")),
					},
				},
			},
			wantErr: errors.New("unknown pathType for path /ccc"),
		},
		{
			name: "Paths contain all three pathTypes",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
					{
						Path:     "/aaa/bbb",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/abc",
						PathType: (*networking.PathType)(awssdk.String("Prefix")),
					},
					{
						Path:     "/ccc",
						PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
					},
				},
			},
			wantExactPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
			wantPrefixPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa/bbb",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
				{
					Path:     "/abc",
					PathType: (*networking.PathType)(awssdk.String("Prefix")),
				},
			},
			wantImplementationSpecificPaths: []networking.HTTPIngressPath{
				{
					Path:     "/ccc",
					PathType: (*networking.PathType)(awssdk.String("ImplementationSpecific")),
				},
			},
		},
		{
			name: "only exact path",
			args: args{
				paths: []networking.HTTPIngressPath{
					{
						Path:     "/aaa",
						PathType: (*networking.PathType)(awssdk.String("Exact")),
					},
				},
			},
			wantExactPaths: []networking.HTTPIngressPath{
				{
					Path:     "/aaa",
					PathType: (*networking.PathType)(awssdk.String("Exact")),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			gotExactPaths, gotPrefixPaths, gotImplementationSpecificPaths, err := task.classifyIngressPathsByType(tt.args.paths)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, gotExactPaths, tt.wantExactPaths)
				assert.Equal(t, gotPrefixPaths, tt.wantPrefixPaths)
				assert.Equal(t, gotImplementationSpecificPaths, tt.wantImplementationSpecificPaths)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatterns(t *testing.T) {
	pathTypeImplementationSpecific := networking.PathTypeImplementationSpecific
	pathTypeExact := networking.PathTypeExact
	pathTypePrefix := networking.PathTypePrefix
	pathTypeUnknown := networking.PathType("unknown")
	type args struct {
		paths    []string
		pathType *networking.PathType
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/* with empty pathType",
			args: args{
				paths:    []string{"/*"},
				pathType: nil,
			},
			want: []string{"/*"},
		},
		{
			name: "/* with implementationSpecific pathType",
			args: args{
				paths:    []string{"/*"},
				pathType: &pathTypeImplementationSpecific,
			},
			want: []string{"/*"},
		},
		{
			name: "/* with exact pathType",
			args: args{
				paths:    []string{"/*"},
				pathType: &pathTypeExact,
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /*"),
		},
		{
			name: "/* with prefix pathType",
			args: args{
				paths:    []string{"/*"},
				pathType: &pathTypePrefix,
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /*"),
		},
		{
			name: "/abc/ with empty pathType",
			args: args{
				paths:    []string{"/abc/"},
				pathType: nil,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with implementationSpecific pathType",
			args: args{
				paths:    []string{"/abc/"},
				pathType: &pathTypeImplementationSpecific,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with exact pathType",
			args: args{
				paths:    []string{"/abc/"},
				pathType: &pathTypeExact,
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/ with prefix pathType",
			args: args{
				paths:    []string{"/abc/"},
				pathType: &pathTypePrefix,
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/ with unknown pathType",
			args: args{
				paths:    []string{"/abc/"},
				pathType: &pathTypeUnknown,
			},
			wantErr: errors.New("unsupported pathType: unknown"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatterns(tt.args.paths, tt.args.pathType)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildImplementationSpecificPathPatterns(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with implementationSpecific pathType",
			args: args{
				path: "/",
			},
			want: []string{"/"},
		},
		{
			name: "/abc with implementationSpecific pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc"},
		},
		{
			name: "/abc/ with implementationSpecific pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/def with implementationSpecific pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def"},
		},
		{
			name: "/abc/def/ with implementationSpecific pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def/"},
		},
		{
			name: "/* with implementationSpecific pathType",
			args: args{
				path: "/*",
			},
			want: []string{"/*"},
		},
		{
			name: "/? with implementationSpecific pathType",
			args: args{
				path: "/?",
			},
			want: []string{"/?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForImplementationSpecificPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatternsForExactPathType(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with exact pathType",
			args: args{
				path: "/",
			},
			want: []string{"/"},
		},
		{
			name: "/abc with exact pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc"},
		},
		{
			name: "/abc/ with exact pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc/"},
		},
		{
			name: "/abc/def with exact pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def"},
		},
		{
			name: "/abc/def/ with exact pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def/"},
		},
		{
			name: "/* with exact pathType",
			args: args{
				path: "/*",
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /*"),
		},
		{
			name: "/? with exact pathType",
			args: args{
				path: "/?",
			},
			wantErr: errors.New("exact path shouldn't contain wildcards: /?"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForExactPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}

func Test_defaultModelBuildTask_buildPathPatternsForPrefixPathType(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr error
	}{
		{
			name: "/ with prefix pathType",
			args: args{
				path: "/",
			},
			want: []string{"/*"},
		},
		{
			name: "/abc with prefix pathType",
			args: args{
				path: "/abc",
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/ with prefix pathType",
			args: args{
				path: "/abc/",
			},
			want: []string{"/abc", "/abc/*"},
		},
		{
			name: "/abc/def with prefix pathType",
			args: args{
				path: "/abc/def",
			},
			want: []string{"/abc/def", "/abc/def/*"},
		},
		{
			name: "/abc/def/ with prefix pathType",
			args: args{
				path: "/abc/def/",
			},
			want: []string{"/abc/def", "/abc/def/*"},
		},
		{
			name: "/* with prefix pathType",
			args: args{
				path: "/*",
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /*"),
		},
		{
			name: "/? with prefix pathType",
			args: args{
				path: "/?",
			},
			wantErr: errors.New("prefix path shouldn't contain wildcards: /?"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{}
			got, err := task.buildPathPatternsForPrefixPathType(tt.args.path)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, got, tt.want)
			}
		})
	}
}
