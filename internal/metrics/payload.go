package metrics

type MetricsPayload struct {
	SchemaVersion int          `json:"schema_version"`
	HostID        string       `json:"host_id,omitempty"` // Deprecated: use ClientIDs.
	ClientIDs     []string     `json:"-"`
	SampledAtMs   int64        `json:"sampled_at_ms"`
	Mode          string       `json:"mode"`
	GpuCount      int          `json:"gpu_count"`
	GPUs          []MetricsGpu `json:"gpus"`
}

type MetricsGpu struct {
	Index      int     `json:"index"`
	GpuID      string  `json:"gpu_id"`
	GpuModel   string  `json:"gpu_model"`
	ModelName  *string `json:"model_name,omitempty"`
	ComputePct float64 `json:"compute_pct"`
	MemoryPct  float64 `json:"memory_pct"`
	PcieGBs    float64 `json:"pcie_gbs"`
	NvlinkGBs  float64 `json:"nvlink_gbs"`
}

type MetricsResponse struct {
	GpuCeilings []GpuCeilingResponse `json:"gpu_ceilings"`
}

type GpuCeilingResponse struct {
	Index             int      `json:"index"`
	ModelName         *string  `json:"model_name,omitempty"`
	ComputeSolCeiling *float64 `json:"compute_sol_ceiling"`
}
