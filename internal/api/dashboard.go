package api

import (
	"net/http"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// runStatusCounts returns a map of every RunStatus to its occurrence count,
// zero-filled so the dashboard always reports the full status set.
func runStatusCounts(runs []store.Run) map[string]int {
	counts := map[string]int{
		string(store.RunStatusSuccess):    0,
		string(store.RunStatusFailed):     0,
		string(store.RunStatusRolledBack): 0,
		string(store.RunStatusPending):    0,
		string(store.RunStatusRunning):    0,
	}
	for _, r := range runs {
		key := string(r.Status)
		if _, ok := counts[key]; ok {
			counts[key]++
		}
	}
	return counts
}

// taskStatusCounts returns a map of every TaskStatus to its occurrence count,
// zero-filled so the dashboard always reports the full status set.
func taskStatusCounts(tasks []store.Task) map[string]int {
	counts := map[string]int{
		string(store.TaskStatusSuccess): 0,
		string(store.TaskStatusFailed):  0,
		string(store.TaskStatusPending): 0,
		string(store.TaskStatusRunning): 0,
	}
	for _, t := range tasks {
		key := string(t.Status)
		if _, ok := counts[key]; ok {
			counts[key]++
		}
	}
	return counts
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFrom(r)
	agents, _ := s.Store.ListAgents(r.Context(), tenant)
	servers, _ := s.Store.ListMcpServers(r.Context(), tenant)
	flows, _ := s.Store.ListFlows(r.Context(), tenant)
	runs, _ := s.Store.ListRuns(r.Context(), tenant, "")
	tasks, _ := s.Store.ListTasks(r.Context(), tenant)

	onlineAgents := 0
	for _, a := range agents {
		if a.Enabled {
			onlineAgents++
		}
	}
	connectedServers := 0
	for _, srv := range servers {
		if srv.Status == "connected" {
			connectedServers++
		}
	}
	activeFlows := 0
	for _, f := range flows {
		if f.Enabled {
			activeFlows++
		}
	}

	// Prefer the collector's latest sample; fall back to sampling on demand so
	// the dashboard is deterministic even before the first tick.
	sample := s.History.Latest()
	if sample.Time == 0 {
		sample = s.History.Capture()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents": map[string]int{
			"online": onlineAgents,
			"total":  len(agents),
		},
		"mcp_servers": map[string]int{
			"connected": connectedServers,
			"total":     len(servers),
		},
		"active_flows": activeFlows,
		"system": map[string]any{
			"cpu":        sample.CPU,
			"memory":     sample.Memory,
			"goroutines": sample.Goroutines,
		},
		"runs_by_status":    runStatusCounts(runs),
		"tasks_by_status":   taskStatusCounts(tasks),
		"websocket_clients": s.Hub.ClientCount(),
	})
}

func (s *Server) handleDashboardHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"samples": s.History.Snapshot(),
	})
}
