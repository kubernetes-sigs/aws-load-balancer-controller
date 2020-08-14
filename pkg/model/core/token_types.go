package core

import "context"

// Token represent a value that can be resolved at resolution time.
type Token interface {
	// token's value resolution may depends on 0 or more Resources.
	Dependencies() []Resource
}

// StringToken represent a string value that can be resolved at resolution time.
type StringToken interface {
	Token
	Resolve(ctx context.Context) string
}
