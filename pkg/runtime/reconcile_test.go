package runtime

import (
	errmetrics "sigs.k8s.io/aws-load-balancer-controller/pkg/error"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestHandleReconcileError(t *testing.T) {
	type args struct {
		err error
	}

	otherErrType := errors.New("some error")
	wrappedOtherErrorType := &errmetrics.ErrorWithMetrics{
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
				err: &errmetrics.ErrorWithMetrics{},
			},
			want:    ctrl.Result{},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeededAfter",
			args: args{
				err: NewRequeueNeededAfter("some error", 3*time.Second),
			},
			want: ctrl.Result{
				RequeueAfter: 3 * time.Second,
			},
			wantErr: nil,
		},
		{
			name: "input err is RequeueNeeded",
			args: args{
				err: NewRequeueNeeded("some error"),
			},
			want: ctrl.Result{
				Requeue: true,
			},
			wantErr: nil,
		},
		{
			name: "input err is ErrorWithMetrics and is RequeueNeededAfter",
			args: args{
				err: &errmetrics.ErrorWithMetrics{
					Err: NewRequeueNeededAfter("some error", 3*time.Second),
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
				err: &errmetrics.ErrorWithMetrics{
					Err: NewRequeueNeeded("some error"),
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
