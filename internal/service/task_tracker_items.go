package service

// TaskItemRecord stores per-item progress in persisted background tasks.
type TaskItemRecord struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`             // organize / scan / scrape
	Status    string `json:"status"`           // pending / running / succeeded / failed
	Name      string `json:"name"`             // display name (file base name or library name)
	Source    string `json:"source,omitempty"` // source file path (organize)
	DestPath  string `json:"dest_path,omitempty"`
	LibraryID string `json:"library_id,omitempty"`
	Error     string `json:"error,omitempty"`
}
