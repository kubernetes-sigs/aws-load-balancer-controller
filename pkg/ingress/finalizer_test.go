package ingress

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	mock_client "sigs.k8s.io/aws-load-balancer-controller/mocks/controller-runtime/client"
	"testing"
)

func Test_defaultFinalizerManager_AddGroupFinalizer(t *testing.T) {
	type patchIngressCall struct {
		ingInput  networking.Ingress
		ingOutput networking.Ingress
		err       error
	}
	tests := []struct {
		name             string
		groupID          GroupID
		ingListInput     []*networking.Ingress
		ingListOutput    []*networking.Ingress
		patchIngressCall *patchIngressCall
		wantErr          error
	}{
		{
			name: "all Ingress already have finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			ingListInput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
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
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
					},
				},
			},
			ingListOutput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
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
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
					},
				},
			},
			patchIngressCall: nil,
			wantErr:          nil,
		},
		{
			name: "some Ingress don't have finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			ingListInput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
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
						Finalizers:      []string{},
						ResourceVersion: "0001",
					},
				},
			},
			ingListOutput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
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
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0002",
					},
				},
			},
			patchIngressCall: &patchIngressCall{
				ingInput: networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
					},
				},
				ingOutput: networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0002",
					},
				},
				err: nil,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			manager := NewDefaultFinalizerManager(client)
			if tt.patchIngressCall != nil {
				client.EXPECT().Patch(gomock.Any(), gomock.Eq(&tt.patchIngressCall.ingInput), gomock.Any()).SetArg(1, tt.patchIngressCall.ingOutput).Return(tt.patchIngressCall.err)
			}

			err := manager.AddGroupFinalizer(context.Background(), tt.groupID, tt.ingListInput...)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
			assert.Equal(t, tt.ingListOutput, tt.ingListInput)
		})
	}
}

func Test_defaultFinalizerManager_RemoveGroupFinalizer(t *testing.T) {
	type patchIngressCall struct {
		ingInput  networking.Ingress
		ingOutput networking.Ingress
		err       error
	}
	tests := []struct {
		name             string
		groupID          GroupID
		ingListInput     []*networking.Ingress
		ingListOutput    []*networking.Ingress
		patchIngressCall *patchIngressCall
		wantErr          error
	}{
		{
			name: "all Ingress already don't have finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			ingListInput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0001",
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
						Finalizers:      []string{},
						ResourceVersion: "0001",
					},
				},
			},
			ingListOutput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0001",
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
						Finalizers:      []string{},
						ResourceVersion: "0001",
					},
				},
			},
			patchIngressCall: nil,
			wantErr:          nil,
		},
		{
			name: "some Ingress have finalizer",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			ingListInput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0001",
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
						Finalizers:      []string{"alb.ingress.k8s.aws/awesome-group"},
						ResourceVersion: "0001",
					},
				},
			},
			ingListOutput: []*networking.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-a",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0001",
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
						Finalizers:      []string{},
						ResourceVersion: "0002",
					},
				},
			},
			patchIngressCall: &patchIngressCall{
				ingInput: networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0001",
					},
				},
				ingOutput: networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "namespace",
						Name:      "ingress-b",
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":          "alb",
							"alb.ingress.kubernetes.io/group.name": "awesome-group",
						},
						Finalizers:      []string{},
						ResourceVersion: "0002",
					},
				},
				err: nil,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			client := mock_client.NewMockClient(ctrl)
			manager := NewDefaultFinalizerManager(client)
			if tt.patchIngressCall != nil {
				client.EXPECT().Patch(gomock.Any(), gomock.Eq(&tt.patchIngressCall.ingInput), gomock.Any()).SetArg(1, tt.patchIngressCall.ingOutput).Return(tt.patchIngressCall.err)
			}

			err := manager.RemoveGroupFinalizer(context.Background(), tt.groupID, tt.ingListInput...)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
			assert.Equal(t, tt.ingListOutput, tt.ingListInput)
		})
	}
}

func Test_buildGroupFinalizer(t *testing.T) {
	tests := []struct {
		name    string
		groupID GroupID
		want    string
	}{
		{
			name: "explicit group",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      "awesome-group",
			}},
			want: "alb.ingress.k8s.aws/awesome-group",
		},
		{
			name: "implicit group",
			groupID: GroupID{NamespacedName: types.NamespacedName{
				Namespace: "namespace",
				Name:      "ingress",
			}},
			want: "alb.ingress.k8s.aws/namespace.ingress",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGroupFinalizer(tt.groupID)
			assert.Equal(t, tt.want, got)
		})
	}
}
