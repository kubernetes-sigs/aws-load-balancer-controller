package core

import (
	"reflect"
	"sigs.k8s.io/aws-alb-ingress-controller/pkg/model/core/graph"
)

// Stack presents a resource graph, where resources can depend on each other.
type Stack interface {
	// Add a resource into stack.
	AddResource(res Resource)

	// Add a dependency relationship between resources.
	AddDependency(dependee Resource, depender Resource)

	// TopologicalTraversal visits resources in stack in topological order.
	TopologicalTraversal(visitor ResourceVisitor) error
}

func NewDefaultStack() *defaultStack {
	return &defaultStack{
		resources:     make(map[graph.ResourceUID]Resource),
		resourceGraph: graph.NewDefaultResourceGraph(),
	}
}

// default implementation for stack.
type defaultStack struct {
	resources     map[graph.ResourceUID]Resource
	resourceGraph graph.ResourceGraph
}

// Add a resource.
func (s *defaultStack) AddResource(res Resource) {
	resUID := s.computeResourceUID(res)
	s.resources[resUID] = res
	s.resourceGraph.AddNode(resUID)
}

// Add a dependency relationship between resources.
func (s *defaultStack) AddDependency(dependee Resource, depender Resource) {
	dependeeResUID := s.computeResourceUID(dependee)
	dependerResUID := s.computeResourceUID(depender)
	s.resourceGraph.AddEdge(dependeeResUID, dependerResUID)
}

func (s *defaultStack) TopologicalTraversal(visitor ResourceVisitor) error {
	return graph.TopologicalTraversal(s.resourceGraph, func(uid graph.ResourceUID) error {
		return visitor.Visit(s.resources[uid])
	})
}

// computeResourceUID returns the UID for resources.
func (s *defaultStack) computeResourceUID(res Resource) graph.ResourceUID {
	return graph.ResourceUID{
		ResType: reflect.TypeOf(res),
		ResID:   res.ID(),
	}
}
