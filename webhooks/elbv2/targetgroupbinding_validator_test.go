package elbv2

import (
	"context"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	elbv2api "sigs.k8s.io/aws-load-balancer-controller/apis/elbv2/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

func Test_targetGroupBindingValidator_ValidateCreate(t *testing.T) {
	type args struct {
		obj *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "targetType is not set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "targetType is set",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: &log.NullLogger{},
			}
			err := v.ValidateCreate(context.Background(), tt.args.obj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_ValidateUpdate(t *testing.T) {
	instanceTargetType := elbv2api.TargetTypeInstance
	type args struct {
		obj    *elbv2api.TargetGroupBinding
		oldObj *elbv2api.TargetGroupBinding
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "tgb updated removes TargetType",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "tgb updated mutates TargetGroupARN",
			args: args{
				obj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
				oldObj: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: &log.NullLogger{},
			}
			err := v.ValidateUpdate(context.Background(), tt.args.obj, tt.args.oldObj)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkRequiredFields(t *testing.T) {
	type args struct {
		tgb *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "targetType is not set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding must specify these fields: spec.targetType"),
		},
		{
			name: "targetType is set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: &log.NullLogger{},
			}
			err := v.checkRequiredFields(tt.args.tgb)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_targetGroupBindingValidator_checkImmutableFields(t *testing.T) {
	type args struct {
		tgb    *elbv2api.TargetGroupBinding
		oldTGB *elbv2api.TargetGroupBinding
	}
	instanceTargetType := elbv2api.TargetTypeInstance
	ipTargetType := elbv2api.TargetTypeIP
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "targetGroupARN is changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &instanceTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN"),
		},
		{
			name: "targetType is changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "targetType is changed from unset to set",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "targetType is changed from set to unset",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     nil,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetType"),
		},
		{
			name: "both targetGroupARN and targetType are changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-2",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &instanceTargetType,
					},
				},
			},
			wantErr: errors.New("TargetGroupBinding update may not change these fields: spec.targetGroupARN,spec.targetType"),
		},
		{
			name: "both targetGroupARN and targetType are not changed",
			args: args{
				tgb: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
				oldTGB: &elbv2api.TargetGroupBinding{
					Spec: elbv2api.TargetGroupBindingSpec{
						TargetGroupARN: "tg-1",
						TargetType:     &ipTargetType,
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &targetGroupBindingValidator{
				logger: &log.NullLogger{},
			}
			err := v.checkImmutableFields(tt.args.tgb, tt.args.oldTGB)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
