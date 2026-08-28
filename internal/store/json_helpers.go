package store

import "encoding/json"

// JSON column marshaling helpers. pgx accepts []byte for json/jsonb columns;
// these helpers keep nil vs empty-slice encoding deterministic (empty slices
// are serialized as `[]`, never `null`).

func skillsJSON(skills []Skill) []byte {
	if skills == nil {
		skills = []Skill{}
	}
	b, _ := json.Marshal(skills)
	return b
}

func permissionsJSON(p FlowPermissionSet) []byte {
	b, _ := json.Marshal(p)
	return b
}

func logsJSON(logs []LogEntry) []byte {
	if logs == nil {
		logs = []LogEntry{}
	}
	b, _ := json.Marshal(logs)
	return b
}

func subtasksJSON(subtasks []Subtask) []byte {
	if subtasks == nil {
		subtasks = []Subtask{}
	}
	b, _ := json.Marshal(subtasks)
	return b
}
