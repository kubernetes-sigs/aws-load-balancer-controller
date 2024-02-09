package throttle

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/client/metadata"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/appmesh"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func Test_NewThrottler(t *testing.T) {
	config := ServiceOperationsThrottleConfig{
		value: map[string][]throttleConfig{
			appmesh.ServiceID: {
				{
					operationPtn: regexp.MustCompile("^Describe"),
					r:            4.2,
					burst:        5,
				},
				{
					operationPtn: regexp.MustCompile("CreateMesh"),
					r:            3.8,
					burst:        4,
				},
			},
			servicediscovery.ServiceID: {
				{
					operationPtn: regexp.MustCompile("^Create"),
					r:            1.2,
					burst:        2,
				},
			},
		},
	}

	throttler := NewThrottler(&config)
	assert.Equal(t, 3, len(throttler.conditionLimiters))
}

func Test_throttler_WithConditionThrottle(t *testing.T) {
	throttler := &throttler{}
	throttler.WithConditionThrottle(matchService(appmesh.ServiceID), 5.0, 10)

	assert.Equal(t, 1, len(throttler.conditionLimiters))

	cl := throttler.conditionLimiters[0]
	assert.True(t, cl.condition(&request.Request{ClientInfo: metadata.ClientInfo{ServiceID: appmesh.ServiceID}}))
	assert.Equal(t, rate.NewLimiter(5.0, 10), cl.limiter)
}

func Test_throttler_WithServiceThrottle(t *testing.T) {
	throttler := &throttler{}
	throttler.WithServiceThrottle(appmesh.ServiceID, 5.0, 10)

	assert.Equal(t, 1, len(throttler.conditionLimiters))

	cl := throttler.conditionLimiters[0]
	assert.True(t, cl.condition(&request.Request{ClientInfo: metadata.ClientInfo{ServiceID: appmesh.ServiceID}}))
	assert.Equal(t, rate.NewLimiter(5.0, 10), cl.limiter)
}

func Test_throttler_WithOperationThrottle(t *testing.T) {
	throttler := &throttler{}
	throttler.WithOperationThrottle(appmesh.ServiceID, "CreateMesh", 5.0, 10)

	assert.Equal(t, 1, len(throttler.conditionLimiters))

	cl := throttler.conditionLimiters[0]
	assert.True(t, cl.condition(&request.Request{
		ClientInfo: metadata.ClientInfo{ServiceID: appmesh.ServiceID},
		Operation:  &request.Operation{Name: "CreateMesh"},
	}))
	assert.Equal(t, rate.NewLimiter(5.0, 10), cl.limiter)
}

func Test_throttler_WithOperationPatternThrottle(t *testing.T) {
	throttler := &throttler{}
	throttler.WithOperationPatternThrottle(appmesh.ServiceID, regexp.MustCompile("^Create"), 5.0, 10)

	assert.Equal(t, 1, len(throttler.conditionLimiters))

	cl := throttler.conditionLimiters[0]
	assert.True(t, cl.condition(&request.Request{
		ClientInfo: metadata.ClientInfo{ServiceID: appmesh.ServiceID},
		Operation:  &request.Operation{Name: "CreateMesh"},
	}))
	assert.Equal(t, rate.NewLimiter(5.0, 10), cl.limiter)
}

func Test_throttler_InjectHandlers(t *testing.T) {
	throttler := &throttler{}
	handlers := request.Handlers{}
	throttler.InjectHandlers(&handlers)
	assert.Equal(t, 1, handlers.Sign.Len())
}

// Test beforeSign to check whether throttle applies correctly.
// Note: the validCallsCount checks whether the observed calls falls into [ideal-1, ideal+1]
// it shouldn't be too precisely to avoid false alarms caused by CPU load when running tests.
// structure your limits and testQPS, so that the expect QPS with/without throttle differs dramatically. (e.g. 10x)
func Test_throttler_beforeSign(t *testing.T) {
	type fields struct {
		conditionLimiters []conditionLimiter
	}
	type args struct {
		r *request.Request
	}
	tests := []struct {
		name            string
		fields          fields
		args            args
		testDuration    time.Duration
		testQPS         int64
		validCallsCount func(elapsedDuration time.Duration, observedCallsCount int64)
	}{
		{
			name: "[single matching condition] throttle should applies",
			fields: fields{
				conditionLimiters: []conditionLimiter{
					{
						condition: func(r *request.Request) bool {
							return true
						},
						limiter: rate.NewLimiter(10, 5),
					},
				},
			},
			args: args{
				r: &request.Request{
					HTTPRequest: &http.Request{},
				},
			},
			testQPS: 100,
			validCallsCount: func(elapsedDuration time.Duration, count int64) {
				ideal := 5 + 10*elapsedDuration.Seconds()
				// We should never get more requests than allowed.
				if want := int64(ideal * 1.1); count > want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
				// We should get very close to the number of requests allowed.
				if want := int64(ideal * 0.9); count < want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
			},
		},
		{
			name: "[single non-matching condition] throttle shouldn't applies",
			fields: fields{
				conditionLimiters: []conditionLimiter{
					{
						condition: func(r *request.Request) bool {
							return false
						},
						limiter: rate.NewLimiter(10, 5),
					},
				},
			},
			args: args{
				r: &request.Request{
					HTTPRequest: &http.Request{},
				},
			},
			testQPS: 100,
			validCallsCount: func(elapsedDuration time.Duration, count int64) {
				ideal := 100 * elapsedDuration.Seconds()
				// We should never get more requests than allowed.
				if want := int64(ideal * 1.1); count > want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
				// We should get very close to the number of requests allowed.
				if want := int64(ideal * 0.9); count < want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
			},
		},
		{
			name: "[two condition, one matching and another non-matching] matching throttle should applies",
			fields: fields{
				conditionLimiters: []conditionLimiter{
					{
						condition: func(r *request.Request) bool {
							return true
						},
						limiter: rate.NewLimiter(10, 5),
					},
					{
						condition: func(r *request.Request) bool {
							return false
						},
						limiter: rate.NewLimiter(1, 5),
					},
				},
			},
			args: args{
				r: &request.Request{
					HTTPRequest: &http.Request{},
				},
			},
			testQPS: 100,
			validCallsCount: func(elapsedDuration time.Duration, count int64) {
				ideal := 5 + 10*elapsedDuration.Seconds()
				// We should never get more requests than allowed.
				if want := int64(ideal * 1.1); count > want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
				// We should get very close to the number of requests allowed.
				if want := int64(ideal * 0.9); count < want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
			},
		},
		{
			name: "[two condition, both matching] most restrictive throttle should applies",
			fields: fields{
				conditionLimiters: []conditionLimiter{
					{
						condition: func(r *request.Request) bool {
							return true
						},
						limiter: rate.NewLimiter(10, 5),
					},
					{
						condition: func(r *request.Request) bool {
							return true
						},
						limiter: rate.NewLimiter(1, 5),
					},
				},
			},
			args: args{
				r: &request.Request{
					HTTPRequest: &http.Request{},
				},
			},
			testQPS: 100,
			validCallsCount: func(elapsedDuration time.Duration, count int64) {
				ideal := 5 + 1*elapsedDuration.Seconds()
				// We should never get more requests than allowed.
				if want := int64(ideal * 1.1); count > want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
				// We should get very close to the number of requests allowed.
				if want := int64(ideal * 0.9); count < want {
					t.Errorf("count = %d, want %d (ideal %f", count, want, ideal)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t1 *testing.T) {
			throttler := &throttler{
				conditionLimiters: tt.fields.conditionLimiters,
			}

			ctx, cancel := context.WithCancel(context.Background())
			tt.args.r.SetContext(ctx)

			observedCount := int64(0)
			start := time.Now()
			end := start.Add(time.Second * 1)
			testQPSThrottle := time.Tick(time.Second / time.Duration(tt.testQPS))
			var wg sync.WaitGroup
			for time.Now().Before(end) {
				wg.Add(1)
				go func() {
					throttler.beforeSign(tt.args.r)
					atomic.AddInt64(&observedCount, 1)
					wg.Done()
				}()
				<-testQPSThrottle
			}
			elapsed := time.Since(start)
			tt.validCallsCount(elapsed, atomic.LoadInt64(&observedCount))
			cancel()
			wg.Wait()
		})
	}
}
