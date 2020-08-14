package core

// Resource represents a deployment unit.
type Resource interface {
	// resource's ID within stack.
	ID() string
}
