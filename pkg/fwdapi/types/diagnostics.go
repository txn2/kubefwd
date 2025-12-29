package types

import "time"

// === Diagnostic Types ===

// DiagnosticSummary provides system-wide diagnostic information
type DiagnosticSummary struct {
	Status          string              `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp       time.Time           `json:"timestamp"`
	Uptime          string              `json:"uptime"`
	Version         string              `json:"version"`
	Services        ServicesSummaryDiag `json:"services"`
	Network         NetworkStatus       `json:"network"`
	Errors          []ErrorDetail       `json:"errors,omitempty"`
	Recommendations []string            `json:"recommendations,omitempty"`
}

// ServicesSummaryDiag provides service counts by status
type ServicesSummaryDiag struct {
	Total   int `json:"total"`
	Active  int `json:"active"`
	Error   int `json:"error"`
	Partial int `json:"partial"`
	Pending int `json:"pending"`
}

// NetworkStatus provides network interface information
type NetworkStatus struct {
	LoopbackInterface string   `json:"loopbackInterface"`
	IPsAllocated      int      `json:"ipsAllocated"`
	IPRange           string   `json:"ipRange"`
	PortsInUse        int      `json:"portsInUse"`
	Hostnames         []string `json:"hostnames,omitempty"`
}

// ServiceDiagnostic provides detailed diagnostics for a single service
type ServiceDiagnostic struct {
	Key            string              `json:"key"`
	ServiceName    string              `json:"serviceName"`
	Namespace      string              `json:"namespace"`
	Context        string              `json:"context"`
	Status         string              `json:"status"`
	Headless       bool                `json:"headless"`
	ActiveCount    int                 `json:"activeCount"`
	ErrorCount     int                 `json:"errorCount"`
	ReconnectState ReconnectState      `json:"reconnectState"`
	SyncState      SyncState           `json:"syncState"`
	Forwards       []ForwardDiagnostic `json:"forwards,omitempty"`
	ErrorHistory   []ErrorDetail       `json:"errorHistory,omitempty"`
}

// ForwardDiagnostic provides detailed diagnostics for a single port forward
type ForwardDiagnostic struct {
	Key           string           `json:"key"`
	ServiceKey    string           `json:"serviceKey"`
	PodName       string           `json:"podName"`
	ContainerName string           `json:"containerName,omitempty"`
	Status        string           `json:"status"`
	Error         string           `json:"error,omitempty"`
	LocalIP       string           `json:"localIP"`
	LocalPort     string           `json:"localPort"`
	PodPort       string           `json:"podPort"`
	Hostnames     []string         `json:"hostnames"`
	ConnectedAt   time.Time        `json:"connectedAt,omitempty"`
	Uptime        string           `json:"uptime,omitempty"`
	LastActive    time.Time        `json:"lastActive,omitempty"`
	IdleDuration  string           `json:"idleDuration,omitempty"`
	Connection    ConnectionStatus `json:"connection"`
	BytesIn       uint64           `json:"bytesIn"`
	BytesOut      uint64           `json:"bytesOut"`
	RateIn        float64          `json:"rateIn"`
	RateOut       float64          `json:"rateOut"`
}

// ReconnectState provides reconnection state machine information
type ReconnectState struct {
	AutoReconnectEnabled bool      `json:"autoReconnectEnabled"`
	CurrentBackoff       string    `json:"currentBackoff"`
	MaxBackoff           string    `json:"maxBackoff"`
	AttemptCount         int       `json:"attemptCount"`
	LastAttempt          time.Time `json:"lastAttempt,omitempty"`
	NextAttempt          time.Time `json:"nextAttempt,omitempty"`
	IsReconnecting       bool      `json:"isReconnecting"`
}

// SyncState provides pod sync state information
type SyncState struct {
	LastSyncedAt time.Time `json:"lastSyncedAt,omitempty"`
	PodCount     int       `json:"podCount"`
	ReadyPods    int       `json:"readyPods"`
}

// ConnectionStatus provides connection health information
type ConnectionStatus struct {
	State        string    `json:"state"` // "connected", "disconnected", "connecting", "error"
	BytesIn      uint64    `json:"bytesIn"`
	BytesOut     uint64    `json:"bytesOut"`
	LastActivity time.Time `json:"lastActivity,omitempty"`
	IdleDuration string    `json:"idleDuration,omitempty"`
}

// ErrorDetail provides detailed error information
type ErrorDetail struct {
	Timestamp   time.Time `json:"timestamp"`
	Component   string    `json:"component"` // "connection", "k8s", "hosts", "network"
	ServiceKey  string    `json:"serviceKey,omitempty"`
	ForwardKey  string    `json:"forwardKey,omitempty"`
	PodName     string    `json:"podName,omitempty"`
	Message     string    `json:"message"`
	Recoverable bool      `json:"recoverable"`
}
