package core

// Stack presents a resource graph, where resources can depend on each other.
type Stack interface {
	// Add a resource.
	AddResource(res Resource)

	// Add a dependency relationship between resources.
	AddDependency(src Resource, dst Resource)
}
