package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConsoleServer serves the migration console web UI and API endpoints.
type ConsoleServer struct {
	k8sClient client.Client
	namespace string
}

// NewConsoleServer creates a new ConsoleServer.
func NewConsoleServer(k8sClient client.Client, namespace string) *ConsoleServer {
	return &ConsoleServer{
		k8sClient: k8sClient,
		namespace: namespace,
	}
}

// Handler returns an http.Handler with all routes registered.
// Routes:
//   - GET /              serves the HTML page (Gateway selector + comparison UI)
//   - GET /api/gateways  returns JSON list of all Gateways with dry-run plans (used by UI to render tabs)
//   - GET /api/diff      returns field-level diff JSON for a specific Gateway (used by UI when a tab is selected)
func (s *ConsoleServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/gateways", s.handleGateways)
	mux.HandleFunc("/api/diff", s.handleDiff)
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleGateways returns all Gateways with dry-run plans in the namespace.
func (s *ConsoleServer) handleGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gateways, err := DiscoverGateways(ctx, s.k8sClient, s.namespace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type gatewayListItem struct {
		Name              string      `json:"name"`
		Namespace         string      `json:"namespace"`
		IngressPlanHolder string      `json:"ingressPlanHolder"`
		Error             string      `json:"error,omitempty"`
		Summary           DiffSummary `json:"summary"`
	}

	items := make([]gatewayListItem, 0, len(gateways))
	for _, gw := range gateways {
		item := gatewayListItem{
			Name:              gw.Name,
			Namespace:         gw.Namespace,
			IngressPlanHolder: gw.IngressPlanHolder,
			Error:             gw.Error,
		}
		// Compute summary if both plans are available.
		if gw.IngressPlan != "" && gw.GatewayPlan != "" {
			inTree, err1 := ParseStack(gw.IngressPlan)
			gwTree, err2 := ParseStack(gw.GatewayPlan)
			if err1 == nil && err2 == nil {
				diff := Diff(inTree, gwTree)
				item.Summary = diff.Summary
			}
		}
		items = append(items, item)
	}

	writeJSON(w, items)
}

// handleDiff returns the field-level diff for a specific Gateway.
func (s *ConsoleServer) handleDiff(w http.ResponseWriter, r *http.Request) {
	gatewayName := r.URL.Query().Get("gateway")
	if gatewayName == "" {
		writeError(w, http.StatusBadRequest, "missing ?gateway= parameter")
		return
	}

	info, err := s.loadGatewayWithFallback(r.Context(), gatewayName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if info.IngressPlan == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no ingress plan found for gateway %s", gatewayName))
		return
	}

	inTree, err := ParseStack(info.IngressPlan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse ingress plan: %v", err))
		return
	}
	gwTree, err := ParseStack(info.GatewayPlan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse gateway plan: %v", err))
		return
	}

	diff := Diff(inTree, gwTree)
	diff.IngressSource = info.IngressPlanHolder
	diff.GatewaySource = fmt.Sprintf("%s/%s", info.Namespace, info.Name)

	writeJSON(w, diff)
}

// TO-DO: handleIndex serves the placeholder HTML page.
func (s *ConsoleServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>LBC Migration Console</title></head>
<body>
<h1>LBC Migration Console</h1>
<p>Namespace: %s</p>
<p>API endpoints:</p>
<ul>
  <li><a href="/api/gateways">/api/gateways</a> — list all Gateways with dry-run plans</li>
  <li>/api/diff?gateway=NAME — field-level diff for a Gateway</li>
</ul>
<p>Full web UI under development...</p>
</body>
</html>`, s.namespace)
}

func (s *ConsoleServer) loadGatewayWithFallback(ctx context.Context, gatewayName string) (*GatewayInfo, error) {
	return LoadGatewayInfo(ctx, s.k8sClient, s.namespace, gatewayName)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
