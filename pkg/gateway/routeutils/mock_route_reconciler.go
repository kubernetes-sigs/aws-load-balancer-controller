package routeutils

type EnqueuedType = struct{ RouteData RouteData }

type MockRouteReconciler struct {
	Enqueued []EnqueuedType
}

// NewMockRouteReconciler creates a new mock reconciler
func NewMockRouteReconciler() *MockRouteReconciler {
	return &MockRouteReconciler{
		Enqueued: make([]EnqueuedType, 0),
	}
}

func (m *MockRouteReconciler) Enqueue(routeData RouteData) {
	m.Enqueued = append(m.Enqueued, EnqueuedType{
		routeData,
	})
}

func (m *MockRouteReconciler) Run() {
	// No-op for test implementation
}
