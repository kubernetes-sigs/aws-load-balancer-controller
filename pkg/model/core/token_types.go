package core

import (
	"context"
	"fmt"
)

// Token represent a value that can be resolved at resolution time.
type Token interface {
	// token's value resolution may depends on 0 or more Resources.
	Dependencies() []Resource
}

// StringToken represent a string value that can be resolved at resolution time.
type StringToken interface {
	Token
	Resolve(ctx context.Context) (string, error)
}

var _ StringToken = LiteralStringToken("")

// LiteralStringToken represents a literal string value.
type LiteralStringToken string

func (t LiteralStringToken) Resolve(ctx context.Context) (string, error) {
	return string(t), nil
}

func (t LiteralStringToken) Dependencies() []Resource {
	return nil
}

// NewResourceFieldStringToken constructs new ResourceFieldStringToken.
// @TODO: ideally the resolverFunc can be a shared implementation which dump Resource as json and obtain the fieldPath.
func NewResourceFieldStringToken(res Resource, fieldPath string,
	resolverFunc func(ctx context.Context, res Resource, fieldPath string) (string, error)) *ResourceFieldStringToken {
	return &ResourceFieldStringToken{
		res:         res,
		fieldPath:   fieldPath,
		resolveFunc: resolverFunc,
	}
}

var _ StringToken = &ResourceFieldStringToken{}

type ResourceFieldStringToken struct {
	res         Resource
	fieldPath   string
	resolveFunc func(ctx context.Context, res Resource, fieldPath string) (string, error)
}

func (t *ResourceFieldStringToken) Resolve(ctx context.Context) (string, error) {
	return t.resolveFunc(ctx, t.res, t.fieldPath)
}

func (t *ResourceFieldStringToken) Dependencies() []Resource {
	return []Resource{t.res}
}

func (t *ResourceFieldStringToken) MarshalJSON() ([]byte, error) {
	payload := fmt.Sprintf(`{"$ref": "#/resources/%v/%v/%v"}`, t.res.Type(), t.res.ID(), t.fieldPath)
	return []byte(payload), nil
}
