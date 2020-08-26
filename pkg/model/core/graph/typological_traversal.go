package graph

import (
	"github.com/pkg/errors"
)

// TopologicalTraversal will traversal nodes in typological order.
// @TODO: change this traversal to be parallel.
func TopologicalTraversal(graph ResourceGraph, visitFunc func(uid ResourceUID) error) error {
	nodes := graph.Nodes()
	indegreeByNode := make(map[ResourceUID]int, len(nodes))
	for _, node := range nodes {
		if _, ok := indegreeByNode[node]; !ok {
			indegreeByNode[node] = 0
		}
		for _, outEdgeNode := range graph.OutEdgeNodes(node) {
			indegreeByNode[outEdgeNode]++
		}

	}

	var queue []ResourceUID
	for node, indegree := range indegreeByNode {
		if indegree == 0 {
			queue = append(queue, node)
		}
	}

	for len(queue) > 0 {
		node := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if err := visitFunc(node); err != nil {
			return err
		}

		for _, outEdgeNode := range graph.OutEdgeNodes(node) {
			indegreeByNode[outEdgeNode]--
			if indegreeByNode[outEdgeNode] == 0 {
				queue = append(queue, outEdgeNode)
			}
		}
	}

	for _, indegree := range indegreeByNode {
		if indegree > 0 {
			return errors.New("ResourceGraph is not a DAG")
		}
	}
	return nil
}
