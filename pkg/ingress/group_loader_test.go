package ingress

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/equality"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_defaultGroupLoader_Load(t *testing.T) {
	now := metav1.Date(2021, 03, 28, 11, 11, 11, 0, time.UTC)
	ingClassA := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-a",
		},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networking.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "ing-class-a-params",
			},
		},
	}
	ingClassAParams := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-a-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			Group: &elbv2api.IngressGroup{
				Name: "awesome-group",
			},
		},
	}

	ingClassB := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-b",
		},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networking.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "ing-class-b-params",
			},
		},
	}

	ingClassBParams := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-b-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			Group: &elbv2api.IngressGroup{
				Name: "awesome-group",
			},
		},
	}

	ingClassC := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-c",
		},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
			Parameters: &networking.IngressClassParametersReference{
				APIGroup: awssdk.String("elbv2.k8s.aws"),
				Kind:     "IngressClassParams",
				Name:     "ing-class-c-params",
			},
		},
	}

	ingClassCParams := &elbv2api.IngressClassParams{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-c-params",
		},
		Spec: elbv2api.IngressClassParamsSpec{
			Group: &elbv2api.IngressGroup{
				Name: "another-group",
			},
		},
	}

	ingClassD := &networking.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ing-class-d",
		},
		Spec: networking.IngressClassSpec{
			Controller: "ingress.k8s.aws/alb",
		},
	}

	ing1 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-1",
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassA.Name),
		},
	}

	ing1BeenDeletedWithoutFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ing-ns",
			Name:              "ing-1",
			DeletionTimestamp: &now,
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassA.Name),
		},
	}
	ing1BeenDeletedWithFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-1",
			Finalizers: []string{
				"group.ingress.k8s.aws/awesome-group",
			},
			DeletionTimestamp: &now,
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassA.Name),
		},
	}
	ing1WithHighGroupOrder := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-1",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/group.order": "100",
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassA.Name),
		},
	}

	ing2 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-2",
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassB.Name),
		},
	}
	ing3 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-3",
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassC.Name),
		},
	}
	ing4 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-4",
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassD.Name),
		},
	}
	ing5 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-5",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/group.name": "awesome-group",
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassD.Name),
		},
	}
	ing5WithImplicitGroupFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-5",
			Annotations: map[string]string{
				"alb.ingress.kubernetes.io/group.name": "awesome-group",
			},
			Finalizers: []string{
				"ingress.k8s.aws/resources",
			},
		},
		Spec: networking.IngressSpec{
			IngressClassName: awssdk.String(ingClassD.Name),
		},
	}

	ing6 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-6",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "alb",
			},
		},
	}
	ing6BeenDeletedWithoutFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-6",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "alb",
			},
			DeletionTimestamp: &now,
		},
	}
	ing6BeenDeletedWithFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-6",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "alb",
			},
			Finalizers: []string{
				"ingress.k8s.aws/resources",
			},
			DeletionTimestamp: &now,
		},
	}
	ing7 := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-7",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":          "alb",
				"alb.ingress.kubernetes.io/group.name": "awesome-group",
			},
		},
	}
	ing7WithImplicitGroupFinalizer := &networking.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ing-ns",
			Name:      "ing-7",
			Annotations: map[string]string{
				"kubernetes.io/ingress.class":          "alb",
				"alb.ingress.kubernetes.io/group.name": "awesome-group",
			},
			Finalizers: []string{
				"ingress.k8s.aws/resources",
			},
		},
	}

	type env struct {
		ingClassList       []*networking.IngressClass
		ingClassParamsList []*elbv2api.IngressClassParams
		ingList            []*networking.Ingress
	}
	type args struct {
		groupID GroupID
	}
	tests := []struct {
		name    string
		env     env
		args    args
		want    Group
		wantErr error
	}{
		{
			name: "load explicit group(awesome-group)",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Name: "awesome-group"},
			},
			want: Group{
				ID: GroupID{Name: "awesome-group"},
				Members: []ClassifiedIngress{
					{
						Ing: ing1,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassA,
							IngClassParams: ingClassAParams,
						},
					},
					{
						Ing: ing2,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassB,
							IngClassParams: ingClassBParams,
						},
					},
					{
						Ing: ing5,
						IngClassConfig: ClassConfiguration{
							IngClass: ingClassD,
						},
					},
					{
						Ing:            ing7,
						IngClassConfig: ClassConfiguration{},
					},
				},
				InactiveMembers: nil,
			},
		},
		{
			name: "load explicit group(awesome-group) - ing-1 been deleted with finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1BeenDeletedWithFinalizer, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Name: "awesome-group"},
			},
			want: Group{
				ID: GroupID{Name: "awesome-group"},
				Members: []ClassifiedIngress{
					{
						Ing: ing2,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassB,
							IngClassParams: ingClassBParams,
						},
					},
					{
						Ing: ing5,
						IngClassConfig: ClassConfiguration{
							IngClass: ingClassD,
						},
					},
					{
						Ing:            ing7,
						IngClassConfig: ClassConfiguration{},
					},
				},
				InactiveMembers: []*networking.Ingress{
					ing1BeenDeletedWithFinalizer,
				},
			},
		},
		{
			name: "load explicit group(awesome-group) - ing-1 been deleted without finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1BeenDeletedWithoutFinalizer, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Name: "awesome-group"},
			},
			want: Group{
				ID: GroupID{Name: "awesome-group"},
				Members: []ClassifiedIngress{
					{
						Ing: ing2,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassB,
							IngClassParams: ingClassBParams,
						},
					},
					{
						Ing: ing5,
						IngClassConfig: ClassConfiguration{
							IngClass: ingClassD,
						},
					},
					{
						Ing:            ing7,
						IngClassConfig: ClassConfiguration{},
					},
				},
				InactiveMembers: nil,
			},
		},
		{
			name: "load explicit group(awesome-group) - ing-1 have explicit high group order",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1WithHighGroupOrder, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Name: "awesome-group"},
			},
			want: Group{
				ID: GroupID{Name: "awesome-group"},
				Members: []ClassifiedIngress{
					{
						Ing: ing2,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassB,
							IngClassParams: ingClassBParams,
						},
					},
					{
						Ing: ing5,
						IngClassConfig: ClassConfiguration{
							IngClass: ingClassD,
						},
					},
					{
						Ing:            ing7,
						IngClassConfig: ClassConfiguration{},
					},
					{
						Ing: ing1WithHighGroupOrder,
						IngClassConfig: ClassConfiguration{
							IngClass:       ingClassA,
							IngClassParams: ingClassAParams,
						},
					},
				},
				InactiveMembers: nil,
			},
		},
		{
			name: "load implicit group(ing-ns/ing-4)",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-4"},
			},
			want: Group{
				ID: GroupID{Namespace: "ing-ns", Name: "ing-4"},
				Members: []ClassifiedIngress{
					{
						Ing: ing4,
						IngClassConfig: ClassConfiguration{
							IngClass: ingClassD,
						},
					},
				},
				InactiveMembers: nil,
			},
		},
		{
			name: "load implicit group(ing-ns/ing-6)",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-6"},
			},
			want: Group{
				ID: GroupID{Namespace: "ing-ns", Name: "ing-6"},
				Members: []ClassifiedIngress{
					{
						Ing:            ing6,
						IngClassConfig: ClassConfiguration{},
					},
				},
				InactiveMembers: nil,
			},
		},
		{
			name: "load implicit group(ing-ns/ing-6) - ing-6 been deleted without finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6BeenDeletedWithoutFinalizer, ing7,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-6"},
			},
			want: Group{
				ID:              GroupID{Namespace: "ing-ns", Name: "ing-6"},
				Members:         nil,
				InactiveMembers: nil,
			},
		},
		{
			name: "load implicit group(ing-ns/ing-6) - ing-6 been deleted with finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6BeenDeletedWithFinalizer, ing7,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-6"},
			},
			want: Group{
				ID:              GroupID{Namespace: "ing-ns", Name: "ing-6"},
				Members:         nil,
				InactiveMembers: []*networking.Ingress{ing6BeenDeletedWithFinalizer},
			},
		},
		{
			name: "load implicit group(ing-ns/ing-7) - ing-7 without implicit group finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5, ing6, ing7,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-7"},
			},
			want: Group{
				ID:              GroupID{Namespace: "ing-ns", Name: "ing-7"},
				Members:         nil,
				InactiveMembers: nil,
			},
		},
		{
			name: "load implicit group(ing-ns/ing-7) - ing-7 with implicit group finalizer",
			env: env{
				ingClassList: []*networking.IngressClass{
					ingClassA, ingClassB, ingClassC, ingClassD,
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					ingClassAParams, ingClassBParams, ingClassCParams,
				},
				ingList: []*networking.Ingress{
					ing1, ing2, ing3, ing4, ing5WithImplicitGroupFinalizer, ing6, ing7WithImplicitGroupFinalizer,
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-7"},
			},
			want: Group{
				ID:      GroupID{Namespace: "ing-ns", Name: "ing-7"},
				Members: nil,
				InactiveMembers: []*networking.Ingress{
					ing7WithImplicitGroupFinalizer,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClassList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClass.DeepCopy()))
			}
			for _, ingClassParams := range tt.env.ingClassParamsList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClassParams.DeepCopy()))
			}
			for _, ing := range tt.env.ingList {
				assert.NoError(t, k8sClient.Create(context.Background(), ing.DeepCopy()))
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classLoader := NewDefaultClassLoader(k8sClient)
			classAnnotationMatcher := NewDefaultClassAnnotationMatcher("alb")
			m := &defaultGroupLoader{
				client:                             k8sClient,
				annotationParser:                   annotationParser,
				classLoader:                        classLoader,
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: false,
			}
			got, err := m.Load(context.Background(), tt.args.groupID)
			if tt.wantErr != nil {
				assert.Equal(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
				}
				assert.True(t, cmp.Equal(tt.want, got, opt),
					"diff: %v", cmp.Diff(tt.want, got, opt))
			}
		})
	}
}

func Test_defaultGroupLoader_LoadGroupIDsPendingFinalization(t *testing.T) {
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name string
		args args
		want []GroupID
	}{
		{
			name: "one finalizer for explicit group",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Finalizers: []string{
							"group.ingress.k8s.aws/awesome-group",
						},
					},
				},
			},
			want: []GroupID{
				{
					Name: "awesome-group",
				},
			},
		},
		{
			name: "one finalizer for implicit group",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Finalizers: []string{
							"ingress.k8s.aws/resources",
						},
					},
				},
			},
			want: []GroupID{
				{
					Namespace: "ing-ns",
					Name:      "ing-name",
				},
			},
		},
		{
			name: "one finalizer for explicit group",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Finalizers: []string{
							"group.ingress.k8s.aws/awesome-group",
						},
					},
				},
			},
			want: []GroupID{
				{
					Name: "awesome-group",
				},
			},
		},
		{
			name: "multiple finalizer for IngressGroups",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Finalizers: []string{
							"group.ingress.k8s.aws/awesome-group-1",
							"group.ingress.k8s.aws/awesome-group-2",
							"ingress.k8s.aws/resources",
							"some-group/some-finalizer",
						},
					},
				},
			},
			want: []GroupID{
				{
					Name: "awesome-group-1",
				},
				{
					Name: "awesome-group-2",
				},
				{
					Namespace: "ing-ns",
					Name:      "ing-name",
				},
			},
		},
		{
			name: "no finalizer",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: nil,
					},
				},
			},
			want: nil,
		},
		{
			name: "no finalizer for IngressGroups",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Finalizers: []string{
							"some-group/some-finalizer",
						},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &defaultGroupLoader{}
			got := m.LoadGroupIDsPendingFinalization(context.Background(), tt.args.ing)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_defaultGroupLoader_isGroupMember(t *testing.T) {
	now := metav1.Now()
	type env struct {
		ingClassList       []*networking.IngressClass
		ingClassParamsList []*elbv2api.IngressClassParams
	}
	type args struct {
		groupID GroupID
		ing     *networking.Ingress
	}
	tests := []struct {
		name              string
		env               env
		args              args
		wantClassifiedIng ClassifiedIngress
		wantIsGroupMember bool
		wantErr           error
	}{
		{
			name: "ingress is member of explicit group - groupID match with groupName from IngressClassParams",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			args: args{
				groupID: GroupID{Name: "awesome-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			wantIsGroupMember: true,
		},
		{
			name: "ingress isn't member of explicit group - groupID mismatch with groupName from IngressClassParams",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			args: args{
				groupID: GroupID{Name: "another-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			wantIsGroupMember: false,
		},
		{
			name: "ingress is member of explicit group - groupID match with groupName from annotation",
			args: args{
				groupID: GroupID{Name: "awesome-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIsGroupMember: true,
		},
		{
			name: "ingress isn't member of explicit group - groupID mismatch with groupName from annotation",
			args: args{
				groupID: GroupID{Name: "another-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIsGroupMember: false,
		},
		{
			name: "ingress is member of implicit group - with ingressClassName",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: nil,
						},
					},
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-name"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: nil,
						},
					},
				},
			},
			wantIsGroupMember: true,
		},
		{
			name: "ingress isn't member of implicit group - with ingressClassName",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: nil,
						},
					},
				},
			},
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "another-ing-name"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: nil,
						},
					},
				},
			},
			wantIsGroupMember: false,
		},
		{
			name: "ingress is member of implicit group - with ingressClass annotation",
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "ing-name"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIsGroupMember: true,
		},
		{
			name: "ingress isn't member of implicit group - with ingressClass annotation",
			args: args{
				groupID: GroupID{Namespace: "ing-ns", Name: "another-ing-name"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIsGroupMember: false,
		},
		{
			name: "ingress isn't member of a explicit group - invalid IngressClass",
			args: args{
				groupID: GroupID{Name: "awesome-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ingress-class-non-exists"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantIsGroupMember: false,
		},
		{
			name: "ingress isn't member of a explicit group - invalid IngressGroup",
			args: args{
				groupID: GroupID{Name: "awesome-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group$",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantIsGroupMember: false,
		},
		{
			name: "ingress isn't member of a explicit group - ingress-been-deleted",
			args: args{
				groupID: GroupID{Name: "awesome-group"},
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						DeletionTimestamp: &now,
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantIsGroupMember: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClassList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClass.DeepCopy()))
			}
			for _, ingClassParams := range tt.env.ingClassParamsList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClassParams.DeepCopy()))
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classLoader := NewDefaultClassLoader(k8sClient)
			classAnnotationMatcher := NewDefaultClassAnnotationMatcher("alb")
			m := &defaultGroupLoader{
				client:                             k8sClient,
				annotationParser:                   annotationParser,
				classLoader:                        classLoader,
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: false,
			}

			gotClassifiedIng, gotIsGroupMember, err := m.isGroupMember(context.Background(), tt.args.groupID, tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
				}
				assert.True(t, cmp.Equal(tt.wantClassifiedIng, gotClassifiedIng, opt),
					"diff: %v", cmp.Diff(tt.wantClassifiedIng, gotClassifiedIng, opt))
				assert.Equal(t, tt.wantIsGroupMember, gotIsGroupMember)
			}
		})
	}
}

func Test_defaultGroupLoader_loadGroupIDIfAnyHelper(t *testing.T) {
	now := metav1.Now()
	awesomeGroupID := GroupID{Name: "awesome-group"}
	ingImplicitGroupID := GroupID{Namespace: "ing-ns", Name: "ing-name"}

	type env struct {
		ingClassList       []*networking.IngressClass
		ingClassParamsList []*elbv2api.IngressClassParams
	}

	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name              string
		env               env
		args              args
		wantClassifiedIng ClassifiedIngress
		wantGroupID       *GroupID
		wantErr           error
	}{
		{
			name: "ingress no longer belong to any IngressGroup when it's been deleted",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
						DeletionTimestamp: &now,
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantGroupID:       nil,
			wantErr:           nil,
		},
		{
			name: "ingress specified groupID via IngressClassParams",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
				},
				ingClassParamsList: []*elbv2api.IngressClassParams{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
							Parameters: &networking.IngressClassParametersReference{
								APIGroup: awssdk.String("elbv2.k8s.aws"),
								Kind:     "IngressClassParams",
								Name:     "ing-class-params",
							},
						},
					},
					IngClassParams: &elbv2api.IngressClassParams{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class-params",
						},
						Spec: elbv2api.IngressClassParamsSpec{
							Group: &elbv2api.IngressGroup{
								Name: "awesome-group",
							},
						},
					},
				},
			},
			wantGroupID: &awesomeGroupID,
			wantErr:     nil,
		},
		{
			name: "ingress specified groupID via annotation",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantGroupID: &awesomeGroupID,
			wantErr:     nil,
		},
		{
			name: "ingress defaults to have implicit groupID",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantGroupID: &ingImplicitGroupID,
			wantErr:     nil,
		},
		{
			name: "ingress failed to be classified",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantGroupID:       nil,
			wantErr:           errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"ing-class\" not found"),
		},
		{
			name: "ingress isn't matched by controller's class",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantGroupID:       nil,
			wantErr:           nil,
		},
		{
			name: "ingress's groupID is invalid",
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group$",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{},
			wantGroupID:       nil,
			wantErr:           errors.New("invalid ingress group: groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClassList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClass.DeepCopy()))
			}
			for _, ingClassParams := range tt.env.ingClassParamsList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClassParams.DeepCopy()))
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classLoader := NewDefaultClassLoader(k8sClient)
			classAnnotationMatcher := NewDefaultClassAnnotationMatcher("alb")
			m := &defaultGroupLoader{
				client:                             k8sClient,
				annotationParser:                   annotationParser,
				classLoader:                        classLoader,
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: false,
			}
			gotClassifiedIng, gotGroupID, err := m.loadGroupIDIfAnyHelper(context.Background(), tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
				}
				assert.True(t, cmp.Equal(tt.wantClassifiedIng, gotClassifiedIng, opt),
					"diff: %v", cmp.Diff(tt.wantClassifiedIng, gotClassifiedIng, opt))
				assert.Equal(t, tt.wantGroupID, gotGroupID)
			}
		})
	}
}

func Test_defaultGroupLoader_classifyIngress(t *testing.T) {
	type env struct {
		ingClassList []*networking.IngressClass
	}
	type fields struct {
		ingressClass                       string
		manageIngressesWithoutIngressClass bool
	}
	type args struct {
		ing *networking.Ingress
	}
	tests := []struct {
		name                    string
		env                     env
		fields                  fields
		args                    args
		wantClassifiedIng       ClassifiedIngress
		wantIngressClassMatches bool
		wantErr                 error
	}{
		{
			name: "class specified via annotation - matches",
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIngressClassMatches: true,
		},
		{
			name: "class specified via annotation - mismatches",
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "nginx",
						},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIngressClassMatches: false,
		},
		{
			name: "class specified via ingressClassName - matches",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			wantIngressClassMatches: true,
		},
		{
			name: "class specified via both annotation & ingressClassName - annotation takes priority",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "ingress.k8s.aws/alb",
						},
					},
				},
			},
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ing-ns",
						Name:      "ing-name",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class": "alb",
						},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIngressClassMatches: true,
		},
		{
			name: "class specified via ingressClassName - mismatches",
			env: env{
				ingClassList: []*networking.IngressClass{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "some.other/nginx",
						},
					},
				},
			},
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{
					IngClass: &networking.IngressClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: "ing-class",
						},
						Spec: networking.IngressClassSpec{
							Controller: "some.other/nginx",
						},
					},
				},
			},
			wantIngressClassMatches: false,
		},
		{
			name: "class specified via ingressClassName - ingressClass not found",
			env: env{
				ingClassList: []*networking.IngressClass{},
			},
			fields: fields{
				ingressClass: "alb",
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
					Spec: networking.IngressSpec{
						IngressClassName: awssdk.String("ing-class"),
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantErr: errors.New("invalid ingress class: ingressclasses.networking.k8s.io \"ing-class\" not found"),
		},
		{
			name: "no class specified - manageIngressesWithoutIngressClass is set",
			fields: fields{
				ingressClass:                       "",
				manageIngressesWithoutIngressClass: true,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIngressClassMatches: true,
		},
		{
			name: "no class specified - manageIngressesWithoutIngressClass isn't set",
			fields: fields{
				ingressClass:                       "",
				manageIngressesWithoutIngressClass: false,
			},
			args: args{
				ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
				},
			},
			wantClassifiedIng: ClassifiedIngress{
				Ing: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   "ing-ns",
						Name:        "ing-name",
						Annotations: map[string]string{},
					},
				},
				IngClassConfig: ClassConfiguration{},
			},
			wantIngressClassMatches: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			for _, ingClass := range tt.env.ingClassList {
				assert.NoError(t, k8sClient.Create(context.Background(), ingClass.DeepCopy()))
			}

			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			classLoader := NewDefaultClassLoader(k8sClient)
			classAnnotationMatcher := NewDefaultClassAnnotationMatcher(tt.fields.ingressClass)
			m := &defaultGroupLoader{
				client:                             k8sClient,
				annotationParser:                   annotationParser,
				classLoader:                        classLoader,
				classAnnotationMatcher:             classAnnotationMatcher,
				manageIngressesWithoutIngressClass: tt.fields.manageIngressesWithoutIngressClass,
			}

			gotClassifiedIng, gotIngressClassMatches, err := m.classifyIngress(context.Background(), tt.args.ing)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				opt := cmp.Options{
					equality.IgnoreFakeClientPopulatedFields(),
				}
				assert.True(t, cmp.Equal(tt.wantClassifiedIng, gotClassifiedIng, opt),
					"diff: %v", cmp.Diff(tt.wantClassifiedIng, gotClassifiedIng, opt))
				assert.Equal(t, tt.wantIngressClassMatches, gotIngressClassMatches)
			}
		})
	}
}

func Test_defaultGroupLoader_loadGroupID(t *testing.T) {
	type args struct {
		classifiedIng ClassifiedIngress
	}
	tests := []struct {
		name    string
		args    args
		want    GroupID
		wantErr error
	}{
		{
			name: "groupName specified via Ingress annotation",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ing-ns",
							Name:      "ing-name",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
						},
					},
				},
			},
			want: GroupID{Name: "awesome-group"},
		},
		{
			name: "groupName specified via Ingress annotation - but invalid",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ing-ns",
							Name:      "ing-name",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "awesome-group$",
							},
						},
					},
				},
			},
			wantErr: errors.New("invalid ingress group: groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		},
		{
			name: "groupName specified via IngressClassParams",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "ing-ns",
							Name:        "ing-name",
							Annotations: map[string]string{},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							Spec: elbv2api.IngressClassParamsSpec{
								Group: &elbv2api.IngressGroup{
									Name: "awesome-group",
								},
							},
						},
					},
				},
			},
			want: GroupID{Name: "awesome-group"},
		},
		{
			name: "groupName specified via IngressClassParams - but invalid",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "ing-ns",
							Name:        "ing-name",
							Annotations: map[string]string{},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							Spec: elbv2api.IngressClassParamsSpec{
								Group: &elbv2api.IngressGroup{
									Name: "awesome-group$",
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("invalid ingress group: groupName must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"),
		},
		{
			name: "groupName specified via both Ingress annotation & IngressClassParams",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ing-ns",
							Name:      "ing-name",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "awesome-group-via-anno",
							},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							Spec: elbv2api.IngressClassParamsSpec{
								Group: &elbv2api.IngressGroup{
									Name: "awesome-group-via-params",
								},
							},
						},
					},
				},
			},
			want: GroupID{Name: "awesome-group-via-params"},
		},
		{
			name: "groupName specified via both Ingress annotation & IngressClassParams",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ing-ns",
							Name:      "ing-name",
							Annotations: map[string]string{
								"alb.ingress.kubernetes.io/group.name": "awesome-group-via-anno",
							},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							Spec: elbv2api.IngressClassParamsSpec{
								Group: &elbv2api.IngressGroup{
									Name: "awesome-group-via-params",
								},
							},
						},
					},
				},
			},
			want: GroupID{Name: "awesome-group-via-params"},
		},
		{
			name: "groupName not specified",
			args: args{
				classifiedIng: ClassifiedIngress{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "ing-ns",
							Name:        "ing-name",
							Annotations: map[string]string{},
						},
					},
					IngClassConfig: ClassConfiguration{
						IngClassParams: &elbv2api.IngressClassParams{
							Spec: elbv2api.IngressClassParamsSpec{
								Group: nil,
							},
						},
					},
				},
			},
			want: GroupID{Namespace: "ing-ns", Name: "ing-name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotationParser := annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io")
			m := &defaultGroupLoader{
				annotationParser: annotationParser,
			}
			got, err := m.loadGroupID(tt.args.classifiedIng)
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
		members []ClassifiedIngress
		want    []ClassifiedIngress
		wantErr error
	}{
		{
			name: "sort implicitly sorted Ingresses",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
		},
		{
			name: "sort explicitly sorted Ingresses",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				},
				{
					Ing: &networking.Ingress{
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
			},
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				},
				{
					Ing: &networking.Ingress{
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
		},
		{
			name: "sort explicitly & implicitly sorted Ingresses",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
		},
		{
			name: "sort single Ingress",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
		},
		{
			name: "invalid group order format",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
			},
			want:    nil,
			wantErr: errors.New("failed to load Ingress group order for ingress: namespace/ingress: failed to parse int64 annotation, alb.ingress.kubernetes.io/group.order: x: strconv.ParseInt: parsing \"x\": invalid syntax"),
		},
		{
			name: "two ingress with the same order should be sorted lexically",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				{
					Ing: &networking.Ingress{
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
				},
			},
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
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
				},
				{
					Ing: &networking.Ingress{
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
			},
		},
		{
			name: "negative orders are allow",
			members: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-b",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":           "alb",
								"alb.ingress.kubernetes.io/group.name":  "awesome-group",
								"alb.ingress.kubernetes.io/group.order": "0",
							},
						},
					},
				},
				{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":           "alb",
								"alb.ingress.kubernetes.io/group.name":  "awesome-group",
								"alb.ingress.kubernetes.io/group.order": "-1",
							},
						},
					},
				},
			},
			want: []ClassifiedIngress{
				{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":           "alb",
								"alb.ingress.kubernetes.io/group.name":  "awesome-group",
								"alb.ingress.kubernetes.io/group.order": "-1",
							},
						},
					},
				},
				{
					Ing: &networking.Ingress{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-b",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":           "alb",
								"alb.ingress.kubernetes.io/group.name":  "awesome-group",
								"alb.ingress.kubernetes.io/group.order": "0",
							},
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
			got, err := m.sortGroupMembers(tt.members)
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
