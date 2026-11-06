package wcp

type SupervisorSummaryList struct {
	Supervisors []SupervisorSummary `json:"items"`
}

type SupervisorSummary struct {
	Supervisor string `json:"supervisor"`
	Info       Info   `json:"info"`
}

type Info struct {
	Stats            Stats  `json:"stats"`
	KubernetesStatus string `json:"kubernetes_status"`
	Name             string `json:"name"`
	Messages         any    `json:"messages"`
	ConfigStatus     string `json:"config_status"`
}

type Stats struct {
	CPUUsed         int64 `json:"cpu_used"`
	StorageCapacity int64 `json:"storage_capacity"`
	MemoryUsed      int64 `json:"memory_used"`
	CPUCapacity     int64 `json:"cpu_capacity"`
	MemoryCapacity  int64 `json:"memory_capacity"`
	StorageUsed     int64 `json:"storage_used"`
}
