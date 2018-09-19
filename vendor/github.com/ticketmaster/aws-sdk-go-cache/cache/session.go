package cache

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"time"

	"github.com/ticketmaster/aws-sdk-go-cache/timing"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
)

type contextKeyType int

var cacheContextKey = new(contextKeyType)

type cacheObject struct {
	body io.ReadCloser
	req  *request.Request
}

var cacheHandler = request.NamedHandler{
	Name: "cache.cacheHandler",
	Fn: func(r *request.Request) {
		cacheConfig := getConfig(r.HTTPRequest.Context())
		i := cacheConfig.get(r.ClientInfo.ServiceName, r.Operation.Name, r.Params)

		if i != nil && !i.Expired() {
			v := i.Value().(*cacheObject)

			// Copy cached data to this request
			r.HTTPResponse = v.req.HTTPResponse
			r.HTTPResponse.Body = v.body

			// set value in context to mark that this is a cached result
			r.HTTPRequest = r.HTTPRequest.WithContext(context.WithValue(r.HTTPRequest.Context(), cacheContextKey, true))

			// Adjust start time of HTTP request since the httptrace ConnectStart will not be executed
			td := timing.GetData(r.HTTPRequest.Context())
			if td != nil {
				td.SetConnectionStart(time.Now())
			}
		} else {
			// Cache a copy of the HTTP response body
			bodyBytes, _ := ioutil.ReadAll(r.HTTPResponse.Body)
			cacheBody := ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
			r.HTTPResponse.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

			o := &cacheObject{
				body: cacheBody,
				req:  r,
			}

			cacheConfig.set(r.ClientInfo.ServiceName, r.Operation.Name, r.Params, o)
		}
	},
}

var useCacheHandlerListRunItem = func(item request.HandlerListRunItem) bool {
	cacheConfig := getConfig(item.Request.HTTPRequest.Context())
	i := cacheConfig.get(item.Request.ClientInfo.ServiceName, item.Request.Operation.Name, item.Request.Params)
	if i != nil && !i.Expired() {
		return false
	}
	return true
}

// AddCaching adds caching to the Session
func AddCaching(s *session.Session, cacheConfig *Config) {
	s.Handlers.Send.AfterEachFn = useCacheHandlerListRunItem
	s.Handlers.ValidateResponse.PushFrontNamed(cacheHandler)
	s.Handlers.Validate.PushFront(func(r *request.Request) {
		r.HTTPRequest = r.HTTPRequest.WithContext(context.WithValue(r.HTTPRequest.Context(), configContextKey, cacheConfig))
	})
}

// IsCacheHit returns true if the context was used for a cached API request
func IsCacheHit(ctx context.Context) bool {
	cached := ctx.Value(cacheContextKey)
	return cached != nil
}
