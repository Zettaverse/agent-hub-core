package api

import (
	"net/http"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func countEnabledAgents(agents []store.Agent) int {
	n := 0
	for _, a := range agents {
		if a.Enabled {
			n++
		}
	}
	return n
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFrom(r)
	agents, _ := s.Store.ListAgents(r.Context(), tenant)
	servers, _ := s.Store.ListMcpServers(r.Context(), tenant)
	flows, _ := s.Store.ListFlows(r.Context(), tenant)
	runs, _ := s.Store.ListRuns(r.Context(), tenant, "")
	tasks, _ := s.Store.ListTasks(r.Context(), tenant)

	var runsByStatus = map[string]int{}
	for _, run := range runs {
		runsByStatus[string(run.Status)]++
	}
	var tasksByStatus = map[string]int{}
	for _, task := range tasks {
		tasksByStatus[string(task.Status)]++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents":            len(agents),
		"enabled_agents":    countEnabledAgents(agents),
		"mcp_servers":       len(servers),
		"flows":             len(flows),
		"runs":              len(runs),
		"runs_by_status":    runsByStatus,
		"tasks":             len(tasks),
		"tasks_by_status":   tasksByStatus,
		"websocket_clients": s.Hub.ClientCount(),
	})
}
