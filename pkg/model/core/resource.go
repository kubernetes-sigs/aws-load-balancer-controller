package core

// Resource represents a deployment unit.
type Resource interface {
	// resource's Type.
	Type() string

	// resource's ID within stack.
	ID() string
}

// ResourceVisitor represents a functor that can operate on a resource.
type ResourceVisitor interface {
	Visit(res Resource) error
}
