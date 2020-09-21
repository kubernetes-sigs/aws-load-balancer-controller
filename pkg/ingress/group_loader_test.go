package ingress

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	mock_client "sigs.k8s.io/aws-alb-ingress-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/annotations"
	"testing"
)

func Test_defaultGroupLoader_FindGroupID(t *testing.T) {
	tests := []struct {
		name    string
		ing     *networking.Ingress
		want    *GroupID
		wantErr error
	}{
		{
			name: "explicit group",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":          "alb",
						"alb.ingress.kubernetes.io/group.name": "awesome-group",
					},
				},
			},
			want: &GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
		},
		{
			name: "implicit group",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
			},
			want: &GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
		},
		{
			name: "ingress class mismatch",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: nil,
		},
		{
			name: "invalid group name",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":          "alb",
						"alb.ingress.kubernetes.io/group.name": "a$b",
					},
				},
			},
			want:    nil,
			wantErr: errors.New(`groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:           client,
				annotationParser: annotationParser,
				ingressClass:     "alb",
			}
			got, err := m.FindGroupID(context.Background(), tt.ing)
			assert.Equal(t, tt.want, got)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_defaultGroupLoader_Load(t *testing.T) {
	now := metav1.Now()

	type listIngressesCall struct {
		ingList networking.IngressList
		err     error
	}

	tests := []struct {
		name              string
		groupID           GroupID
		listIngressesCall *listIngressesCall
		want              Group
		wantErr           error
	}{
		{
			name: "explicit group",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-b",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "cool-group",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "",
					Name:      "awesome-group",
				}},
				Members: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-b",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
						},
					},
				},
			},
		},
		{
			name: "explicit group - deleted Ingress without finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-b",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								DeletionTimestamp: &now,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "cool-group",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "",
					Name:      "awesome-group",
				}},
				Members: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
						},
					},
				},
			},
		},
		{
			name: "explicit group - deleted Ingress with finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-b",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								Finalizers:        []string{"alb.ingress.k8s.aws/awesome-group"},
								DeletionTimestamp: &now,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "cool-group",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "",
					Name:      "awesome-group",
				}},
				Members: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
						},
					},
				},
				InactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-b",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
							Finalizers:        []string{"alb.ingress.k8s.aws/awesome-group"},
							DeletionTimestamp: &now,
						},
					},
				},
			},
		},
		{
			name: "implicit group",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress-a",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "namespace",
					Name:      "ingress-a",
				}},
				Members: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class": "alb",
							},
						},
					},
				},
			},
		},
		{
			name: "implicit group - deleted Ingress",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress-a",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								DeletionTimestamp: &now,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "namespace",
					Name:      "ingress-a",
				}},
				Members: []*networking.Ingress{},
			},
		},
		{
			name: "implicit group - deleted Ingress",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress-a",
			}},
			listIngressesCall: &listIngressesCall{
				ingList: networking.IngressList{
					Items: []networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								Finalizers:        []string{"alb.ingress.k8s.aws/namespace.ingress-a"},
								DeletionTimestamp: &now,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{NamespacedName: types.NamespacedName{
					Namespace: "namespace",
					Name:      "ingress-a",
				}},
				Members: []*networking.Ingress{},
				InactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class": "alb",
							},
							Finalizers:        []string{"alb.ingress.k8s.aws/namespace.ingress-a"},
							DeletionTimestamp: &now,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:           client,
				annotationParser: annotationParser,
				ingressClass:     "alb",
			}
			if tt.listIngressesCall != nil {
				client.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, tt.listIngressesCall.ingList).Return(tt.listIngressesCall.err)
			}
			got, err := m.Load(context.Background(), tt.groupID)
			assert.Equal(t, tt.want, got)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_defaultGroupLoader_matchesIngressClass(t *testing.T) {
	tests := []struct {
		name         string
		ingressClass string
		ing          *networking.Ingress
		want         bool
	}{
		{
			name:         "desire empty ingress class and no ingress class specified",
			ingressClass: "",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: true,
		},
		{
			name:         "desire empty ingress class but ingress class specified",
			ingressClass: "",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
		{
			name:         "desire alb ingress class and alb ingress class specified",
			ingressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
			},
			want: true,
		},
		{
			name:         "desire alb ingress class but no ingress class specified",
			ingressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: false,
		},
		{
			name:         "desire alb ingress class but another ingress class specified",
			ingressClass: "alb",
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultGroupLoader{
				ingressClass: tt.ingressClass,
			}
			got := m.matchesIngressClass(tt.ing)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultGroupLoader_isGroupMember(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name    string
		groupID GroupID
		ing     *networking.Ingress
		want    bool
		wantErr error
	}{
		{
			name: "explicit group member",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":          "alb",
						"alb.ingress.kubernetes.io/group.name": "awesome-group",
					},
				},
			},
			want: true,
		},
		{
			name: "implicit group member",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
			},
			want: true,
		},
		{
			name: "deleted ingress is no longer member",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
					DeletionTimestamp: &now,
				},
			},
			want: false,
		},
		{
			name: "invalid group name",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":          "alb",
						"alb.ingress.kubernetes.io/group.name": "a$b",
					},
				},
			},
			want:    false,
			wantErr: errors.New(`groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:           client,
				annotationParser: annotationParser,
				ingressClass:     "alb",
			}
			got, err := m.isGroupMember(context.Background(), tt.groupID, tt.ing)
			assert.Equal(t, tt.want, got)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_defaultGroupLoader_sortGroupMembers(t *testing.T) {
	tests := []struct {
		name    string
		members []*networking.Ingress
		want    []*networking.Ingress
		wantErr error
	}{
		{
			name: "sort implicitly sorted Ingresses",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			want: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
		},
		{
			name: "sort explicitly sorted Ingresses",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "3",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "2",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "1",
						},
					},
				},
			},
			want: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "2",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "3",
						},
					},
				},
			},
		},
		{
			name: "sort explicitly & implicitly sorted Ingresses",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			want: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-c",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "1",
						},
					},
				},
			},
		},
		{
			name: "sort single Ingress",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			want: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
		},
		{
			name: "invalid group order format",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.order": "x",
						},
					},
				},
			},
			want:    nil,
			wantErr: errors.New("failed to load Ingress group order for ingress: namespace/ingress: failed to parse int64 annotation, alb.ingress.kubernetes.io/group.order: x: strconv.ParseInt: parsing \"x\": invalid syntax"),
		},
		{
			name: "invalid group order range",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.order": "1001",
						},
					},
				},
			},
			want:    nil,
			wantErr: errors.New("explicit Ingress group order must be within [1:1000], Ingress: namespace/ingress, order: 1001"),
		},
		{
			name: "two ingress shouldn't have same explicit order",
			members: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "42",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":           "alb",
							"alb.ingress.kubernetes.io/group.name":  "awesome-group",
							"alb.ingress.kubernetes.io/group.order": "42",
						},
					},
				},
			},
			want:    nil,
			wantErr: errors.New("conflict Ingress group order: 42"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:           client,
				annotationParser: annotationParser,
				ingressClass:     "alb",
			}
			got, err := m.sortGroupMembers(context.Background(), tt.members)
			assert.Equal(t, tt.want, got)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_validateGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   error
	}{
		{
			name:      "pure lower case letters",
			groupName: "group",
			wantErr:   nil,
		},
		{
			name:      "pure numbers",
			groupName: "42",
			wantErr:   nil,
		},
		{
			name:      "lower case letters and numbers",
			groupName: "m00nf1sh",
			wantErr:   nil,
		},
		{
			name:      "lower case letters and numbers and dash",
			groupName: "group-m00nf1sh",
			wantErr:   nil,
		},
		{
			name:      "upper case letters",
			groupName: "GROUP",
			wantErr:   errors.Errorf("groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"),
		},
		{
			name:      "starting with dash",
			groupName: "-abcdef",
			wantErr:   errors.Errorf("groupName must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGroupName(tt.groupName)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}
