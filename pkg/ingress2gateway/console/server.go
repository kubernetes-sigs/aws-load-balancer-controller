package console

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConsoleServer serves the migration console web UI and API endpoints.
// The server operates cluster-wide: it lists namespaces containing Gateways
// with dry-run-plan annotations, then lists gateways and diffs on demand.
type ConsoleServer struct {
	k8sClient client.Client
}

// NewConsoleServer creates a new ConsoleServer.
func NewConsoleServer(k8sClient client.Client) *ConsoleServer {
	return &ConsoleServer{k8sClient: k8sClient}
}

// Handler returns an http.Handler with all routes registered.
// Routes:
//   - GET /                               serves the HTML page (landing page + comparison UI router)
//   - GET /static/                        serves embedded static assets (CSS, JS)
//   - GET /api/namespaces                 returns the namespaces cluster-wide that have at least one
//     Gateway with a dry-run-plan annotation (landing page data)
//   - GET /api/gateways?namespace=<ns>    returns the Gateways in <ns> with dry-run plans + per-gateway summaries
//   - GET /api/diff?namespace=<ns>&gateway=<name>
//     returns the field-level diff for a specific Gateway
func (s *ConsoleServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/namespaces", s.handleNamespaces)
	mux.HandleFunc("/api/gateways", s.handleGateways)
	mux.HandleFunc("/api/diff", s.handleDiff)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleNamespaces returns all namespaces with at least one Gateway that has a dry-run-plan annotation.
func (s *ConsoleServer) handleNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := DiscoverNamespaces(r.Context(), s.k8sClient)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, namespaces)
}

// handleGateways returns all Gateways with dry-run plans in the given namespace.
func (s *ConsoleServer) handleGateways(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeError(w, http.StatusBadRequest, "missing ?namespace= parameter")
		return
	}

	gateways, err := DiscoverGateways(r.Context(), s.k8sClient, namespace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type gatewayListItem struct {
		Name         string      `json:"name"`
		Namespace    string      `json:"namespace"`
		Error        string      `json:"error,omitempty"`
		MigratedFrom string      `json:"migratedFrom,omitempty"`
		Summary      DiffSummary `json:"summary"`
	}

	items := make([]gatewayListItem, 0, len(gateways))
	for _, gw := range gateways {
		item := gatewayListItem{
			Name:         gw.Name,
			Namespace:    gw.Namespace,
			Error:        gw.Error,
			MigratedFrom: gw.MigratedFrom,
		}
		// Compute summary if both plans are available.
		if gw.IngressPlan != "" && gw.GatewayPlan != "" {
			inTree, err1 := ParseStack(gw.IngressPlan)
			gwTree, err2 := ParseStack(gw.GatewayPlan)
			if err1 == nil && err2 == nil {
				userSpecified := BuildUserSpecifiedFields(gw.IngressAnnotations)
				diff := Diff(inTree, gwTree, userSpecified)
				item.Summary = diff.Summary
			}
		}
		items = append(items, item)
	}

	writeJSON(w, items)
}

// handleDiff returns the field-level diff for a specific Gateway in a namespace.
func (s *ConsoleServer) handleDiff(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	gatewayName := r.URL.Query().Get("gateway")
	if namespace == "" {
		writeError(w, http.StatusBadRequest, "missing ?namespace= parameter")
		return
	}
	if gatewayName == "" {
		writeError(w, http.StatusBadRequest, "missing ?gateway= parameter")
		return
	}

	info, err := LoadGatewayInfo(r.Context(), s.k8sClient, namespace, gatewayName)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if info.IngressPlan == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no ingress plan found for gateway %s/%s", namespace, gatewayName))
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

	userSpecified := BuildUserSpecifiedFields(info.IngressAnnotations)
	diff := Diff(inTree, gwTree, userSpecified)

	writeJSON(w, diff)
}

//go:embed static
var staticFiles embed.FS

// handleIndex serves the embedded HTML page.
func (s *ConsoleServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "failed to load UI", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
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
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
