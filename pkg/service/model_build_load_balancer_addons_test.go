// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package service

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/annotations"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/core"
	elbv2model "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/elbv2"
	shieldmodel "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/model/shield"
)

func Test_defaultModelBuildTask_buildShieldProtection(t *testing.T) {
	const shieldAnnotation = "service.beta.kubernetes.io/aws-load-balancer-nlb-shield-advanced-protection"
	nlb := &elbv2model.LoadBalancer{Spec: elbv2model.LoadBalancerSpec{Type: elbv2model.LoadBalancerTypeNetwork}}
	alb := &elbv2model.LoadBalancer{Spec: elbv2model.LoadBalancerSpec{Type: elbv2model.LoadBalancerTypeApplication}}

	type args struct {
		lbARN core.StringToken
	}
	tests := []struct {
		testName string
		svc      *corev1.Service
		lb       *elbv2model.LoadBalancer
		args     args
		want     *shieldmodel.Protection
		wantErr  bool
	}{
		{
			testName: "annotation absent keeps protection unchanged",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			},
			lb:   nlb,
			args: args{lbARN: core.LiteralStringToken("awesome-lb-arn")},
			want: nil,
		},
		{
			testName: "annotation set to true enables protection",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{shieldAnnotation: "true"}},
			},
			lb:   nlb,
			args: args{lbARN: core.LiteralStringToken("awesome-lb-arn")},
			want: &shieldmodel.Protection{
				Spec: shieldmodel.ProtectionSpec{
					Enabled:     true,
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
		},
		{
			testName: "annotation set to false disables protection",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{shieldAnnotation: "false"}},
			},
			lb:   nlb,
			args: args{lbARN: core.LiteralStringToken("awesome-lb-arn")},
			want: &shieldmodel.Protection{
				Spec: shieldmodel.ProtectionSpec{
					Enabled:     false,
					ResourceARN: core.LiteralStringToken("awesome-lb-arn"),
				},
			},
		},
		{
			testName: "non-boolean annotation value is rejected",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{shieldAnnotation: "FalSe1"}},
			},
			lb:      nlb,
			args:    args{lbARN: core.LiteralStringToken("awesome-lb-arn")},
			wantErr: true,
		},
		{
			testName: "annotation on a non-NLB service is rejected",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        "svc",
					Annotations: map[string]string{shieldAnnotation: "true"},
				},
			},
			lb:      alb,
			args:    args{lbARN: core.LiteralStringToken("awesome-lb-arn")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
			task := &defaultModelBuildTask{
				service:          tt.svc,
				annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
				stack:            stack,
				loadBalancer:     tt.lb,
			}
			got, err := task.buildShieldProtection(context.Background(), tt.args.lbARN)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			opts := cmpopts.IgnoreTypes(core.ResourceMeta{})
			assert.True(t, cmp.Equal(tt.want, got, opts), "diff: %s", cmp.Diff(tt.want, got, opts))
		})
	}
}

func Test_defaultModelBuildTask_buildLoadBalancerAddOns(t *testing.T) {
	stack := core.NewDefaultStack(core.StackID{Name: "awesome-stack"})
	task := &defaultModelBuildTask{
		service: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-nlb-shield-advanced-protection": "true",
				},
			},
		},
		annotationParser: annotations.NewSuffixAnnotationParser("service.beta.kubernetes.io"),
		stack:            stack,
		loadBalancer: &elbv2model.LoadBalancer{
			Spec: elbv2model.LoadBalancerSpec{Type: elbv2model.LoadBalancerTypeNetwork},
		},
	}
	lbARN := core.LiteralStringToken("awesome-lb-arn")
	assert.NoError(t, task.buildLoadBalancerAddOns(context.Background(), lbARN))
	// the protection resource was added to the stack
	var protections []*shieldmodel.Protection
	assert.NoError(t, stack.ListResources(&protections))
	assert.Len(t, protections, 1)
	assert.True(t, protections[0].Spec.Enabled)
	assert.Equal(t, lbARN, protections[0].Spec.ResourceARN)
}
