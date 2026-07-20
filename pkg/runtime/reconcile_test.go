package runtime

import (
	ctrlerrors "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/error"
	lbcmetrics "sigs.k8s.io/aws-load-balancer-controller/v3/pkg/metrics/lbc"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestHandleReconcileError(t *testing.T) {
	type args struct {
		err error
	}

	otherErrType := errors.New("some error")
	wrappedOtherErrorType := &ctrlerrors.ErrorWithMetrics{
		Err:          otherErrType,
		ResourceType: "foo",
	}
	tests := []struct {
		name    string
		args    args
		want    ctrl.Result
		wantErr error
	}{
		{
			name: "input err is nil",
			args: args{
				err: nil,
			},
			want:    ctrl.Result{},
			wantErr: nil,
		},
		{
			name: "input err is nil",
			args: args{
				err: &ctrlerrors.ErrorWithMetrics{},
			},
			want:    ctrl.Result{},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeededAfter",
			args: args{
				err: ctrlerrors.NewRequeueNeededAfter("some error", 3*time.Second),
			},
			want: ctrl.Result{
				RequeueAfter: 3 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeeded",
			args: args{
				err: ctrlerrors.NewRequeueNeeded("some error"),
			},
			want: ctrl.Result{
				Requeue: true,
			},
			wantErr: nil,
		},
		{
			name: "input err is ErrorWithMetrics and is RequeueNeededAfter",
			args: args{
				err: &ctrlerrors.ErrorWithMetrics{
					Err: ctrlerrors.NewRequeueNeededAfter("some error", 3*time.Second),
				},
			},
			want: ctrl.Result{
				RequeueAfter: 3 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "input err is ErrorWithMetrics and is RequeueNeeded",
			args: args{
				err: &ctrlerrors.ErrorWithMetrics{
					Err: ctrlerrors.NewRequeueNeeded("some error"),
				},
			},
			want: ctrl.Result{
				Requeue: true,
			},
			wantErr: nil,
		},
		{
			name: "input err is other error type",
			args: args{
				err: otherErrType,
			},
			want:    ctrl.Result{},
			wantErr: otherErrType,
		},
		{
			name: "input err is ErrorWithMetrics with other error type",
			args: args{
				err: wrappedOtherErrorType,
			},
			want:    ctrl.Result{},
			wantErr: wrappedOtherErrorType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HandleReconcileError(tt.args.err, logr.New(&log.NullLogSink{}))
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				assert.Equal(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestHandleReconcileErrorWithCondition(t *testing.T) {
	otherErrType := errors.New("some error")
	tests := []struct {
		name                    string
		err                     error
		want                    ctrl.Result
		wantErr                 error
		wantConditionRecordings int
	}{
		{
			name:                    "input err is nil",
			err:                     nil,
			want:                    ctrl.Result{},
			wantConditionRecordings: 0,
		},
		{
			name:                    "input err is other error type",
			err:                     otherErrType,
			wantErr:                 otherErrType,
			wantConditionRecordings: 1,
		},
		{
			name: "input err is ErrorWithMetrics wrapping other error type",
			err: &ctrlerrors.ErrorWithMetrics{
				Err: otherErrType,
			},
			wantErr:                 otherErrType,
			wantConditionRecordings: 1,
		},
		{
			name:                    "input err is RequeueNeeded",
			err:                     ctrlerrors.NewRequeueNeeded("some error"),
			want:                    ctrl.Result{Requeue: true},
			wantConditionRecordings: 0,
		},
		{
			name: "input err is ErrorWithMetrics wrapping RequeueNeededAfter",
			err: &ctrlerrors.ErrorWithMetrics{
				Err: ctrlerrors.NewRequeueNeededAfter("some error", 3*time.Second),
			},
			want:                    ctrl.Result{RequeueAfter: 3 * time.Second},
			wantConditionRecordings: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := lbcmetrics.NewMockCollector()
			req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "awesome-ns", Name: "my-res"}}
			got, err := HandleReconcileErrorWithCondition(tt.err, "service", req, collector, logr.New(&log.NullLogSink{}))
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			mc := collector.(*lbcmetrics.MockCollector)
			assert.Equal(t, tt.wantConditionRecordings, len(mc.Invocations[lbcmetrics.MetricControllerReconcileCondition]))
		})
	}
}
