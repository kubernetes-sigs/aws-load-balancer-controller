package graph

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func fakeResourceUID(resID string) ResourceUID {
	return ResourceUID{ResID: resID}
}

func Test_defaultResourceGraph_Operations(t *testing.T) {
	tests := []struct {
		name                   string
		operations             func(graph ResourceGraph)
		wantNodes              []ResourceUID
		wantOutEdgeNodesByNode map[ResourceUID][]ResourceUID
	}{
		{
			name: "add a single node",
			operations: func(graph ResourceGraph) {
				graph.AddNode(fakeResourceUID("node-A"))
			},
			wantNodes: []ResourceUID{fakeResourceUID("node-A")},
			wantOutEdgeNodesByNode: map[ResourceUID][]ResourceUID{
				fakeResourceUID("node-A"): nil,
			},
		},
		{
			name: "add two single node, and edge between them",
			operations: func(graph ResourceGraph) {
				graph.AddNode(fakeResourceUID("node-A"))
				graph.AddNode(fakeResourceUID("node-B"))
				graph.AddEdge(fakeResourceUID("node-A"), fakeResourceUID("node-B"))
			},
			wantNodes: []ResourceUID{fakeResourceUID("node-A"), fakeResourceUID("node-B")},
			wantOutEdgeNodesByNode: map[ResourceUID][]ResourceUID{
				fakeResourceUID("node-A"): {fakeResourceUID("node-B")},
				fakeResourceUID("node-B"): nil,
			},
		},
		{
			name: "add three single node - case 1",
			operations: func(graph ResourceGraph) {
				graph.AddNode(fakeResourceUID("node-A"))
				graph.AddNode(fakeResourceUID("node-B"))
				graph.AddNode(fakeResourceUID("node-C"))
				graph.AddEdge(fakeResourceUID("node-A"), fakeResourceUID("node-B"))
				graph.AddEdge(fakeResourceUID("node-B"), fakeResourceUID("node-C"))
			},
			wantNodes: []ResourceUID{fakeResourceUID("node-A"), fakeResourceUID("node-B"), fakeResourceUID("node-C")},
			wantOutEdgeNodesByNode: map[ResourceUID][]ResourceUID{
				fakeResourceUID("node-A"): {fakeResourceUID("node-B")},
				fakeResourceUID("node-B"): {fakeResourceUID("node-C")},
				fakeResourceUID("node-C"): nil,
			},
		},
		{
			name: "add three single node - case 2",
			operations: func(graph ResourceGraph) {
				graph.AddNode(fakeResourceUID("node-A"))
				graph.AddNode(fakeResourceUID("node-B"))
				graph.AddNode(fakeResourceUID("node-C"))
				graph.AddEdge(fakeResourceUID("node-A"), fakeResourceUID("node-B"))
				graph.AddEdge(fakeResourceUID("node-A"), fakeResourceUID("node-C"))
			},
			wantNodes: []ResourceUID{fakeResourceUID("node-A"), fakeResourceUID("node-B"), fakeResourceUID("node-C")},
			wantOutEdgeNodesByNode: map[ResourceUID][]ResourceUID{
				fakeResourceUID("node-A"): {fakeResourceUID("node-B"), fakeResourceUID("node-C")},
				fakeResourceUID("node-B"): nil,
				fakeResourceUID("node-C"): nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewDefaultResourceGraph()
			tt.operations(g)
			assert.Equal(t, tt.wantNodes, g.Nodes())
			for node, wantOutEdgeNodes := range tt.wantOutEdgeNodesByNode {
				gotOutEdgeNodes := g.OutEdgeNodes(node)
				assert.Equal(t, wantOutEdgeNodes, gotOutEdgeNodes)
			}
		})
	}
}
