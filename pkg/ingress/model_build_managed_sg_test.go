package ingress

import (
	"context"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/annotations"
	"testing"
)

func Test_defaultModelBuildTask_buildManagedSecurityGroupTags(t *testing.T) {
	type fields struct {
		ingGroup    Group
		defaultTags map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]string
		wantErr error
	}{
		{
			name: "empty default tags, no tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{},
		},
		{
			name: "empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k1=v1",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k2=v2",
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "non-empty default tags, empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k3": "v3",
				"k4": "v4",
			},
		},
		{
			name: "non-empty default tags, non-empty tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k2=v2",
								},
							},
						},
					},
				},
				defaultTags: map[string]string{
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: map[string]string{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3a",
				"k4": "v4",
			},
		},
		{
			name: "empty default tags, conflicting tags annotation",
			fields: fields{
				ingGroup: Group{
					Members: []*networking.Ingress{
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k1=v1,k3=v3a",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Annotations: map[string]string{
									"alb.ingress.kubernetes.io/tags": "k2=v2,k3=v3b",
								},
							},
						},
					},
				},
				defaultTags: nil,
			},
			wantErr: errors.New("conflicting tag k3: v3a | v3b"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelBuildTask{
				ingGroup:         tt.fields.ingGroup,
				defaultTags:      tt.fields.defaultTags,
				annotationParser: annotations.NewSuffixAnnotationParser("alb.ingress.kubernetes.io"),
			}
			got, err := task.buildManagedSecurityGroupTags(context.Background())
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
