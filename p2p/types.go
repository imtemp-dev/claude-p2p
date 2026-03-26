package p2p

// PeerInfo holds basic information about a peer for JSON output.
type PeerInfo struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name,omitempty"`
	Addrs       []string `json:"addrs"`
	ConnectedAt string   `json:"connected_at,omitempty"`
	Source      string   `json:"source,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Username    string   `json:"username,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Branch      string   `json:"branch,omitempty"`
}
