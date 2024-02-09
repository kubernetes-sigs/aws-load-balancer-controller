package ingress

import (
	"context"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	"testing"
)

func Test_defaultFinalizerManager_AddGroupFinalizer(t *testing.T) {
	type addFinalizersCall struct {
		ing       *networking.Ingress
		finalizer string
		err       error
	}
	type fields struct {
		addFinalizersCalls []addFinalizersCall
	}
	type args struct {
		groupID GroupID
		members []ClassifiedIngress
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "add group finalizer - explicit Group",
			fields: fields{
				addFinalizersCalls: []addFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "group.ingress.k8s.aws/awesome-group",
					},
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-b",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "group.ingress.k8s.aws/awesome-group",
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
				members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								ResourceVersion: "0001",
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
								ResourceVersion: "0001",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "add group finalizer - implicit Group",
			fields: fields{
				addFinalizersCalls: []addFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "ingress.k8s.aws/resources",
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-as",
				},
				members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "add group finalizer - implicit Group - fails",
			fields: fields{
				addFinalizersCalls: []addFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "ingress.k8s.aws/resources",
						err:       errors.New("some-error"),
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-as",
				},
				members: []ClassifiedIngress{
					{
						Ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
					},
				},
			},
			wantErr: errors.New("some-error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
			for _, call := range tt.fields.addFinalizersCalls {
				k8sFinalizerManager.EXPECT().AddFinalizers(gomock.Any(), call.ing, call.finalizer).Return(call.err)
			}

			manager := NewDefaultFinalizerManager(k8sFinalizerManager)
			err := manager.AddGroupFinalizer(context.Background(), tt.args.groupID, tt.args.members)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
		})
	}
}

func Test_defaultFinalizerManager_RemoveGroupFinalizer(t *testing.T) {
	type removeFinalizersCall struct {
		ing       *networking.Ingress
		finalizer string
		err       error
	}
	type fields struct {
		removeFinalizersCalls []removeFinalizersCall
	}
	type args struct {
		groupID         GroupID
		inactiveMembers []*networking.Ingress
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr error
	}{
		{
			name: "remove group finalizer - explicit Group",
			fields: fields{
				removeFinalizersCalls: []removeFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "group.ingress.k8s.aws/awesome-group",
					},
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-b",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class":          "alb",
									"alb.ingress.kubernetes.io/group.name": "awesome-group",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "group.ingress.k8s.aws/awesome-group",
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "",
					Name:      "awesome-group",
				},
				inactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class":          "alb",
								"alb.ingress.kubernetes.io/group.name": "awesome-group",
							},
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
							ResourceVersion: "0001",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "remove group finalizer - implicit Group",
			fields: fields{
				removeFinalizersCalls: []removeFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "ingress.k8s.aws/resources",
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-as",
				},
				inactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class": "alb",
							},
							ResourceVersion: "0001",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "remove group finalizer - implicit Group - fails",
			fields: fields{
				removeFinalizersCalls: []removeFinalizersCall{
					{
						ing: &networking.Ingress{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "namespace",
								Name:      "ingress-a",
								Annotations: map[string]string{
									"kubernetes.io/ingress.class": "alb",
								},
								ResourceVersion: "0001",
							},
						},
						finalizer: "ingress.k8s.aws/resources",
						err:       errors.New("some-error"),
					},
				},
			},
			args: args{
				groupID: GroupID{
					Namespace: "namespace",
					Name:      "ingress-as",
				},
				inactiveMembers: []*networking.Ingress{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "namespace",
							Name:      "ingress-a",
							Annotations: map[string]string{
								"kubernetes.io/ingress.class": "alb",
							},
							ResourceVersion: "0001",
						},
					},
				},
			},
			wantErr: errors.New("some-error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			k8sFinalizerManager := k8s.NewMockFinalizerManager(ctrl)
			for _, call := range tt.fields.removeFinalizersCalls {
				k8sFinalizerManager.EXPECT().RemoveFinalizers(gomock.Any(), call.ing, call.finalizer).Return(call.err)
			}

			manager := NewDefaultFinalizerManager(k8sFinalizerManager)
			err := manager.RemoveGroupFinalizer(context.Background(), tt.args.groupID, tt.args.inactiveMembers)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
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
			groupID: GroupID{
				Namespace: "",
				Name:      "awesome-group",
			},
			want: "group.ingress.k8s.aws/awesome-group",
		},
		{
			name: "implicit group",
			groupID: GroupID{
				Namespace: "namespace",
				Name:      "ingress",
			},
			want: "ingress.k8s.aws/resources",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGroupFinalizer(tt.groupID)
			assert.Equal(t, tt.want, got)
		})
	}
}
