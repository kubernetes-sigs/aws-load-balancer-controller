package cache

import (
	"context"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
)

type contextKeyType int

var cacheHitContextKey = new(contextKeyType)

// IsCacheHit returns true if the context was used for a cached API request
func IsCacheHit(ctx context.Context) bool {
	return ctx.Value(cacheHitContextKey) != nil
}

// AddCaching adds caching to the Session
func AddCaching(s *session.Session, cacheConfig *Config) {
	s.Handlers.Validate.PushFront(func(r *request.Request) {
		// Get item from cache
		i := cacheConfig.get(r)

		if i != nil && !i.Expired() {
			// Add cache hit marker to the request context
			r.HTTPRequest = r.HTTPRequest.WithContext(context.WithValue(r.HTTPRequest.Context(), cacheHitContextKey, true))

			// Set Data to cached value
			r.Data = i.Value()
		}
	})

	// short circuit Send Handlers
	s.Handlers.Send.PushFront(func(r *request.Request) {})
	s.Handlers.Send.AfterEachFn = shortCircuitRequestHandler

	// short circuit ValidateResponse Handlers
	s.Handlers.ValidateResponse.PushFront(func(r *request.Request) {})
	s.Handlers.ValidateResponse.AfterEachFn = shortCircuitRequestHandler

	// short circuit Unmarshal Handlers
	s.Handlers.Unmarshal.PushFront(func(r *request.Request) {})
	s.Handlers.Unmarshal.AfterEachFn = shortCircuitRequestHandler

	s.Handlers.Complete.PushBack(func(r *request.Request) {
		// Cache the processed Data
		if !IsCacheHit(r.HTTPRequest.Context()) {
			cacheConfig.set(r, r.Data)
		}
	})
}

// Returns false when request is a cache hit, used to short circuit request handlers
var shortCircuitRequestHandler = func(item request.HandlerListRunItem) bool {
	return !IsCacheHit(item.Request.HTTPRequest.Context())
}
