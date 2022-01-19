package k8s

import (
	"context"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func TestHasFinalizer(t *testing.T) {
	tests := []struct {
		name      string
		obj       metav1.Object
		finalizer string
		want      bool
	}{
		{
			name: "finalizer exists and matches",
			obj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"alb.ingress.k8s.aws/group"},
				},
			},
			finalizer: "alb.ingress.k8s.aws/group",
			want:      true,
		},
		{
			name: "finalizer not exists",
			obj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{},
				},
			},
			finalizer: "alb.ingress.k8s.aws/group",
			want:      false,
		},
		{
			name: "finalizer exists but not matches",
			obj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"alb.ingress.k8s.aws/group-b"},
				},
			},
			finalizer: "alb.ingress.k8s.aws/group-a",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasFinalizer(tt.obj, tt.finalizer)
			assert.Equal(t, tt.want, got)
		})
	}
}

// IgnoreFakeClientPopulatedFields is an option to ignore fields populated by fakeK8sClient for a comparison.
// Use this when comparing k8s objects in test cases.
// These fields are ignored: TypeMeta and ObjectMeta.ResourceVersion
func IgnoreFakeClientPopulatedFields() cmp.Option {
	return cmp.Options{
		// ignore unset fields in left hand side
		cmpopts.IgnoreTypes(metav1.TypeMeta{}),
		cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"),
	}
}

func Test_defaultFinalizerManager_AddFinalizers(t *testing.T) {
	type args struct {
		obj        *networking.Ingress
		finalizers []string
	}
	tests := []struct {
		name    string
		args    args
		wantObj *networking.Ingress
		wantErr error
	}{
		{
			name: "add one finalizer",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "my-mesh",
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-1"},
				},
			},
		},
		{
			name: "add one finalizer + added finalizer already exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-1"},
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-1"},
				},
			},
		},
		{
			name: "add one finalizer + other finalizer already exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-2"},
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-2", "finalizer-1"},
				},
			},
		},
		{
			name: "add two finalizer",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "my-mesh",
					},
				},
				finalizers: []string{"finalizer-1", "finalizer-2"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-1", "finalizer-2"},
				},
			},
		},
		{
			name: "add two finalizer + one added finalizer already exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-2"},
					},
				},
				finalizers: []string{"finalizer-1", "finalizer-2"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-2", "finalizer-1"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			m := NewDefaultFinalizerManager(k8sClient, &log.NullLogger{})

			err := k8sClient.Create(ctx, tt.args.obj.DeepCopy())
			assert.NoError(t, err)

			err = m.AddFinalizers(ctx, tt.args.obj, tt.args.finalizers...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				gotObj := &networking.Ingress{}
				err = k8sClient.Get(ctx, NamespacedName(tt.args.obj), gotObj)
				assert.NoError(t, err)
				opts := IgnoreFakeClientPopulatedFields()
				assert.True(t, cmp.Equal(tt.wantObj, gotObj, opts), "diff", cmp.Diff(tt.wantObj, gotObj, opts))
			}
		})
	}
}

func Test_defaultFinalizerManager_RemoveFinalizers(t *testing.T) {
	type args struct {
		obj        *networking.Ingress
		finalizers []string
	}
	tests := []struct {
		name    string
		args    args
		wantObj *networking.Ingress
		wantErr error
	}{
		{
			name: "remove one finalizer",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-1"},
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: nil,
				},
			},
		},
		{
			name: "remove one finalizer + removed finalizer didn't exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "my-ns",
						Name:      "my-mesh",
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: nil,
				},
			},
		},
		{
			name: "remove one finalizer + other finalizer already exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-1", "finalizer-2"},
					},
				},
				finalizers: []string{"finalizer-1"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-2"},
				},
			},
		},
		{
			name: "remove two finalizer",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-1", "finalizer-2"},
					},
				},
				finalizers: []string{"finalizer-1", "finalizer-2"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: nil,
				},
			},
		},
		{
			name: "remove two finalizer + one removed finalizer already exists",
			args: args{
				obj: &networking.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "my-ns",
						Name:       "my-mesh",
						Finalizers: []string{"finalizer-2", "finalizer-3"},
					},
				},
				finalizers: []string{"finalizer-1", "finalizer-2"},
			},
			wantObj: &networking.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:  "my-ns",
					Name:       "my-mesh",
					Finalizers: []string{"finalizer-3"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			elbv2api.AddToScheme(k8sSchema)

			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			m := NewDefaultFinalizerManager(k8sClient, &log.NullLogger{})

			err := k8sClient.Create(ctx, tt.args.obj.DeepCopy())
			assert.NoError(t, err)

			err = m.RemoveFinalizers(ctx, tt.args.obj, tt.args.finalizers...)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				gotObj := &networking.Ingress{}
				err = k8sClient.Get(ctx, NamespacedName(tt.args.obj), gotObj)
				assert.NoError(t, err)
				opts := IgnoreFakeClientPopulatedFields()
				assert.True(t, cmp.Equal(tt.wantObj, gotObj, opts), "diff", cmp.Diff(tt.wantObj, gotObj, opts))
			}
		})
	}
}
