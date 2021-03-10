package ingress

import (
	"context"
	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			want: &GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
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
			want: &GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
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
			wantErr: errors.New(`invalid ingress group: groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:                             client,
				annotationParser:                   annotationParser,
				classAnnotationMatcher:             NewDefaultClassAnnotationMatcher(ingressClassALB),
				manageIngressesWithoutIngressClass: false,
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
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
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
				ID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
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
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
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
				ID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
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
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
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
								Finalizers:        []string{"group.ingress.k8s.aws/awesome-group"},
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
				ID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
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
							Finalizers:        []string{"group.ingress.k8s.aws/awesome-group"},
							DeletionTimestamp: &now,
						},
					},
				},
			},
		},
		{
			name: "implicit group",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress-a",
			},
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
				ID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
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
			name: "implicit group - deleted Ingress without finalizer",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress-a",
			},
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
				ID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				Members:         nil,
				InactiveMembers: nil,
			},
		},
		{
			name: "implicit group - deleted Ingress with finalizer",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress-a",
			},
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
								Finalizers:        []string{"ingress.k8s.aws/resources"},
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
								Finalizers: []string{"ingress.k8s.aws/resources"},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				Members: nil,
				InactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class": "alb",
							},
							Finalizers:        []string{"ingress.k8s.aws/resources"},
							DeletionTimestamp: &now,
						},
					},
				},
			},
		},
		{
			name: "implicit group - joined explicit group without finalizer",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress-a",
			},
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
				ID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				Members:         nil,
				InactiveMembers: nil,
			},
		},
		{
			name: "implicit group - joined explicit group with finalizer",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress-a",
			},
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
								Finalizers: []string{"ingress.k8s.aws/resources"},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-c",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								Finalizers: []string{"ingress.k8s.aws/resources"},
							},
						},
					},
				},
			},
			want: Group{
				ID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				Members: nil,
				InactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
							Finalizers: []string{"ingress.k8s.aws/resources"},
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
				client:                             client,
				annotationParser:                   annotationParser,
				classAnnotationMatcher:             NewDefaultClassAnnotationMatcher(ingressClassALB),
				manageIngressesWithoutIngressClass: false,
			}
			if tt.listIngressesCall != nil {
				client.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, tt.listIngressesCall.ingList).Return(tt.listIngressesCall.err)
			}
			got, err := m.Load(context.Background(), tt.groupID)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultGroupLoader_matchesIngressClass(t *testing.T) {
	type env struct {
		ingClasses []*networking.IngressClass
	}
	type fields struct {
		ingressClass                       string
		manageIngressesWithoutIngressClass bool
	}
	tests := []struct {
		name    string
		env     env
		fields  fields
		ing     *networking.Ingress
		want    bool
		wantErr error
	}{
		{
			name: "desire empty ingress class and no ingress class specified",
			fields: fields{
				ingressClass:                       "",
				manageIngressesWithoutIngressClass: true,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: true,
		},
		{
			name: "desire empty ingress class and alb ingress class specified",
			fields: fields{
				ingressClass:                       "",
				manageIngressesWithoutIngressClass: true,
			},
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
			name: "desire empty ingress class but another ingress class specified",
			fields: fields{
				ingressClass:                       "",
				manageIngressesWithoutIngressClass: true,
			},
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
			name: "desire alb ingress class and alb ingress class specified",
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
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
			name: "desire alb ingress class but no ingress class specified",
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			want: false,
		},
		{
			name: "desire alb ingress class but another ingress class specified",
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
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
			name: "ingressClass exists and matches alb controller",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-ing-class"),
				},
			},
			want: true,
		},
		{
			name: "ingressClass exists and matches another controller",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "kubernetes.io/nginx",
						},
					},
				},
			},
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-ing-class"),
				},
			},
			want: false,
		},
		{
			name: "matches alb ingress class via both annotation and spec.ingressClassName",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-ing-class"),
				},
			},
			want: true,
		},
		{
			name: "matches alb ingress class via annotation but not via spec.ingressClassName",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "kubernetes.io/nginx",
						},
					},
				},
			},
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "alb",
					},
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-ing-class"),
				},
			},
			want: true,
		},
		{
			name: "matches alb ingress class via spec.ingressClassName but not via annotation",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			fields: fields{
				ingressClass:                       "alb",
				manageIngressesWithoutIngressClass: false,
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-ing-class"),
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClasses {
				assert.NoError(t, k8sClient.Create(ctx, ingClass.DeepCopy()))
			}

			m := &defaultGroupLoader{
				client:                             k8sClient,
				eventRecorder:                      record.NewFakeRecorder(10),
				classAnnotationMatcher:             NewDefaultClassAnnotationMatcher(tt.fields.ingressClass),
				manageIngressesWithoutIngressClass: tt.fields.manageIngressesWithoutIngressClass,
			}
			got, err := m.matchesIngressClass(ctx, tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultGroupLoader_matchesIngressClassName(t *testing.T) {
	type env struct {
		ingClasses []*networking.IngressClass
	}
	type args struct {
		ingClassName string
	}
	tests := []struct {
		name    string
		env     env
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "ingressClass exists and matches controller",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			args: args{
				ingClassName: "my-ing-class",
			},
			want: true,
		},
		{
			name: "ingressClass exists and mismatches controller",
			env: env{
				ingClasses: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "my-ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "kubernetes.io/nginx",
						},
					},
				},
			},
			args: args{
				ingClassName: "my-ing-class",
			},
			want: false,
		},
		{
			name: "ingressClass doesn't exists",
			env: env{
				ingClasses: nil,
			},
			args: args{
				ingClassName: "my-ing-class",
			},
			want:    false,
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"my-ing-class\" not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClasses {
				assert.NoError(t, k8sClient.Create(ctx, ingClass.DeepCopy()))
			}

			m := &defaultGroupLoader{
				client: k8sClient,
			}
			got, err := m.matchesIngressClassName(ctx, tt.args.ingClassName)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
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
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
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
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
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
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
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
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
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
			want: false,
		},
		{
			name: "invalid ingress class",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
			ing: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace",
					Name:      "ingress",
				},
				Spec: networking.IngressSpec{
					IngressClassName: awssdk.String("my-class"),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				client:                             k8sClient,
				eventRecorder:                      record.NewFakeRecorder(10),
				annotationParser:                   annotationParser,
				classAnnotationMatcher:             NewDefaultClassAnnotationMatcher(ingressClassALB),
				manageIngressesWithoutIngressClass: false,
			}
			got, err := m.isGroupMember(context.Background(), tt.groupID, tt.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func Test_defaultGroupLoader_containsGroupFinalizer(t *testing.T) {
	type args struct {
		groupID   GroupID
		finalizer string
		ing       *networking.Ingress
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "contains explicit group's finalizer",
			args: args{
				groupID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
				finalizer: "group.ingress.k8s.aws/awesome-group",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers: []string{"group.ingress.k8s.aws/awesome-group"},
					},
				},
			},
			want: true,
		},
		{
			name: "doesn't contain explicit group's finalizer",
			args: args{
				groupID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
				finalizer: "group.ingress.k8s.aws/awesome-group",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "other-group",
						},
						Finalizers: []string{"group.ingress.k8s.aws/other-group"},
					},
				},
			},
			want: false,
		},
		{
			name: "contains implicit group's finalizer",
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				finalizer: "ingress.k8s.aws/resources",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
						Finalizers: []string{"ingress.k8s.aws/resources"},
					},
				},
			},
			want: true,
		},
		{
			name: "doesn't contain implicit group's finalizer",
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				finalizer: "ingress.k8s.aws/resources",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
						Finalizers: nil,
					},
				},
			},
			want: false,
		},
		{
			name: "doesn't contain implicit group's finalizer - ingress name doesn't match",
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				finalizer: "ingress.k8s.aws/resources",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
						Finalizers: []string{"ingress.k8s.aws/resources"},
					},
				},
			},
			want: false,
		},
		{
			name: "contains implicit group's finalizer - changed to explicit group",
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				finalizer: "ingress.k8s.aws/resources",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers: []string{"ingress.k8s.aws/resources"},
					},
				},
			},
			want: true,
		},
		{
			name: "doesn't contain implicit group's finalizer - changed to explicit group",
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-a",
				},
				finalizer: "ingress.k8s.aws/resources",
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers: []string{"group.ingress.k8s.aws/awesome-group"},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultGroupLoader{}
			got := m.containsGroupFinalizer(tt.args.groupID, tt.args.finalizer, tt.args.ing)
			assert.Equal(t, tt.want, got)
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
				client:                             client,
				annotationParser:                   annotationParser,
				classAnnotationMatcher:             NewDefaultClassAnnotationMatcher(ingressClassALB),
				manageIngressesWithoutIngressClass: false,
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
			wantErr:   errors.New("groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		},
		{
			name:      "all possible character sets",
			groupName: "aaaa-.cc-c.c",
			wantErr:   nil,
		},
		{
			name:      "starting with dash",
			groupName: "-abcdef",
			wantErr:   errors.New("groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		},
		{
			name:      "63 character length",
			groupName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:   nil,
		},
		{
			name:      "64 character length",
			groupName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:   errors.New("groupName must be no more than 63 characters"),
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
