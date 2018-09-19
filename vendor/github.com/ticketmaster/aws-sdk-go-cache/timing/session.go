package timing

import (
	"context"
	"net/http/httptrace"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
)

type contextKeyType int

var timingContextKey = new(contextKeyType)

var startTraceHandler = request.NamedHandler{
	Name: "timing.StartTraceHandler",
	Fn: func(r *request.Request) {
		trace := &httptrace.ClientTrace{
			GetConn: func(network string) {
				td := GetData(r.HTTPRequest.Context())
				td.SetConnectionStart(time.Now())
			},
		}

		// Add timing data to context
		r.HTTPRequest = r.HTTPRequest.WithContext(context.WithValue(r.HTTPRequest.Context(), timingContextKey, &Data{}))

		// Add tracing to context
		r.HTTPRequest = r.HTTPRequest.WithContext(httptrace.WithClientTrace(r.HTTPRequest.Context(), trace))
	},
}

var finalizeTraceHandler = request.NamedHandler{
	Name: "timing.FinalizeTraceHandler",
	Fn: func(r *request.Request) {
		// TODO: prometheus metrics
	},
}

// AddTiming adds timing measurements to a Session
func AddTiming(s *session.Session) {
	s.Handlers.Sign.PushFrontNamed(startTraceHandler)
	s.Handlers.Complete.PushFrontNamed(finalizeTraceHandler)
}
