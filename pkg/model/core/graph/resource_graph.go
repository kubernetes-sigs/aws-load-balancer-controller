package graph

import "reflect"

// unique ID for a resource.
type ResourceUID struct {
	ResType reflect.Type
	ResID   string
}

// ResourceGraph is an abstraction of resource DAG.
type ResourceGraph interface {
	// Add a node into ResourceGraph.
	AddNode(node ResourceUID)

	// Add a edge into ResourceGraph, where dstNode depends on srcNode.
	AddEdge(srcNode ResourceUID, dstNode ResourceUID)

	// Nodes returns all nodes in ResourceGraph.
	Nodes() []ResourceUID

	// OutEdgeNodes returns all nodes that depends on this node.
	OutEdgeNodes(node ResourceUID) []ResourceUID
}

// NewDefaultResourceGraph constructs new defaultResourceGraph.
func NewDefaultResourceGraph() *defaultResourceGraph {
	return &defaultResourceGraph{
		nodes:    nil,
		outEdges: make(map[ResourceUID][]ResourceUID),
	}
}

var _ ResourceGraph = &defaultResourceGraph{}

// defaultResourceGraph is the default implementation for ResourceGraph.
type defaultResourceGraph struct {
	nodes    []ResourceUID
	outEdges map[ResourceUID][]ResourceUID
}

// Add a node into ResourceGraph.
func (g *defaultResourceGraph) AddNode(node ResourceUID) {
	g.nodes = append(g.nodes, node)
}

// Add a edge into ResourceGraph, where dstNode depends on srcNode.
func (g *defaultResourceGraph) AddEdge(srcNode ResourceUID, dstNode ResourceUID) {
	g.outEdges[srcNode] = append(g.outEdges[srcNode], dstNode)
}

// Nodes returns all nodes in ResourceGraph.
func (g *defaultResourceGraph) Nodes() []ResourceUID {
	return g.nodes
}

// OutEdgeNodes returns all nodes that depends on this node.
func (g *defaultResourceGraph) OutEdgeNodes(node ResourceUID) []ResourceUID {
	return g.outEdges[node]
}
