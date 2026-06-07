package workflow

func DefaultPlaybooks() []Definition {
	return []Definition{
		{
			ID:          "playbook:host-basic-triage",
			Name:        "Host Basic Triage",
			Description: "First-pass service, resource and listener inspection for a host incident.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-host-resources",
					Type: StepRunTool,
					Tool: "template:System Resources",
				},
				{
					ID:   "step-host-memory",
					Type: StepRunTool,
					Tool: "template:Top Memory Processes",
				},
				{
					ID:   "step-host-services",
					Type: StepRunTool,
					Tool: "template:Service Status",
				},
				{
					ID:   "step-host-listeners",
					Type: StepRunTool,
					Tool: "template:Port Scanner",
				},
				{
					ID:     "step-host-summary",
					Type:   StepAskAI,
					Prompt: "Summarize the host state. Highlight resource pressure, suspicious listeners, failed services and the next safest read-only check.",
				},
			},
		},
		{
			ID:          "playbook:linux-service-failure",
			Name:        "Linux Service Failure",
			Description: "Check service state, journal errors, resource pressure and port ownership on Linux.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-linux-services",
					Type: StepRunTool,
					Tool: "template:Service Status",
				},
				{
					ID:   "step-linux-journal-errors",
					Type: StepRunTool,
					Tool: "template:Linux Journal Errors",
				},
				{
					ID:   "step-linux-process-search",
					Type: StepRunTool,
					Tool: "template:Linux Process Search",
					Inputs: map[string]string{
						"ProcessName": "",
					},
				},
				{
					ID:   "step-linux-port-usage",
					Type: StepRunTool,
					Tool: "template:Linux Port Usage",
					Inputs: map[string]string{
						"Port": "",
					},
				},
				{
					ID:     "step-linux-service-summary",
					Type:   StepAskAI,
					Prompt: "Explain the likely Linux service failure mode from the collected output. Separate confirmed facts from guesses and suggest the next read-only command.",
				},
			},
		},
		{
			ID:          "playbook:disk-pressure-triage",
			Name:        "Disk Pressure Triage",
			Description: "Find full filesystems, large directories and large files before cleanup decisions.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-disk-summary",
					Type: StepRunTool,
					Tool: "template:Disk Usage Summary",
				},
				{
					ID:   "step-disk-current-dir",
					Type: StepRunTool,
					Tool: "template:Current Directory Size",
				},
				{
					ID:   "step-largest-directories",
					Type: StepRunTool,
					Tool: "template:Largest Directories",
				},
				{
					ID:   "step-large-files",
					Type: StepRunTool,
					Tool: "template:Find Large Files (by extension)",
				},
				{
					ID:     "step-disk-summary-ai",
					Type:   StepAskAI,
					Prompt: "Rank the most likely disk pressure sources. Recommend safe verification steps before deleting or moving data.",
				},
			},
		},
		{
			ID:          "playbook:api-network-health-check",
			Name:        "API / Network Health Check",
			Description: "Basic DNS, IP, headers and latency checks for an API or network issue.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-api-dns",
					Type: StepRunTool,
					Tool: "template:DNS Lookup",
				},
				{
					ID:   "step-api-ip",
					Type: StepRunTool,
					Tool: "template:Show IP",
				},
				{
					ID:   "step-api-headers",
					Type: StepRunTool,
					Tool: "template:HTTP Headers Check",
				},
				{
					ID:   "step-api-performance",
					Type: StepRunTool,
					Tool: "template:Website Performance",
				},
				{
					ID:     "step-explain-api-health",
					Type:   StepAskAI,
					Prompt: "Summarize the API or network health check and point out the most useful next read-only test.",
				},
			},
		},
		{
			ID:          "playbook:docker-debug",
			Name:        "Docker Debug",
			Description: "Inspect containers, images and recent container logs for Docker troubleshooting.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-docker-ps",
					Type: StepRunTool,
					Tool: "template:Docker Containers",
				},
				{
					ID:   "step-docker-images",
					Type: StepRunTool,
					Tool: "template:Docker Images",
				},
				{
					ID:   "step-docker-container-logs",
					Type: StepRunTool,
					Tool: "template:Docker Container Logs",
					Inputs: map[string]string{
						"Container": "",
					},
				},
				{
					ID:     "step-explain-docker",
					Type:   StepAskAI,
					Prompt: "Analyze the Docker container, image and log output. Identify stopped, restarting or unhealthy containers and suggest next troubleshooting steps.",
				},
			},
		},
		{
			ID:          "playbook:docker-compose-service-incident",
			Name:        "Docker Compose / Container Incident",
			Description: "Run docker ps, choose a running container and inspect its logs.",
			Mode:        ModeApprove,
			Steps: []Step{
				{
					ID:   "step-compose-docker-ps",
					Type: StepRunTool,
					Tool: "template:Docker Containers",
				},
				{
					ID:   "step-compose-container-logs",
					Type: StepRunTool,
					Tool: "template:Docker Container Logs",
					Inputs: map[string]string{
						"Container": "",
					},
				},
				{
					ID:     "step-compose-explain",
					Type:   StepAskAI,
					Prompt: "Explain the docker ps and container log output. Identify the most likely failure mode and suggest the next safe read-only command.",
				},
			},
		},
		{
			ID:          "playbook:k8s-pod-triage-v2",
			Name:        "K8s Pod Triage",
			Description: "Check pod status, pod logs and describe output for a Kubernetes namespace.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-k8s-pods",
					Type: StepRunTool,
					Tool: "template:K8s Get Pods",
					Inputs: map[string]string{
						"Namespace": "default",
					},
				},
				{
					ID:   "step-k8s-pod-logs",
					Type: StepRunTool,
					Tool: "template:K8s Pod Logs",
					Inputs: map[string]string{
						"Namespace": "default",
						"Pod":       "",
					},
				},
				{
					ID:   "step-k8s-describe-pod",
					Type: StepRunTool,
					Tool: "template:K8s Describe Resource",
					Inputs: map[string]string{
						"ResourceType": "pod",
						"Namespace":    "default",
						"ResourceName": "",
					},
				},
				{
					ID:     "step-k8s-explain",
					Type:   StepAskAI,
					Prompt: "Analyze the Kubernetes pod status, logs and describe output. Identify likely causes such as image pull, crash loop, scheduling, probes or config issues.",
				},
			},
		},
		{
			ID:          "playbook:windows-host-triage",
			Name:        "Windows Host Triage",
			Description: "Inspect Windows services, event log errors, disk state and processes.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-windows-errors",
					Type: StepRunTool,
					Tool: "template:Windows Event Log Errors",
				},
				{
					ID:   "step-windows-disk",
					Type: StepRunTool,
					Tool: "template:Windows Disk Report",
				},
				{
					ID:   "step-windows-service",
					Type: StepRunTool,
					Tool: "template:Windows Service Details",
					Inputs: map[string]string{
						"ServiceName": "",
					},
				},
				{
					ID:   "step-windows-process",
					Type: StepRunTool,
					Tool: "template:Windows Process Search",
					Inputs: map[string]string{
						"ProcessName": "",
					},
				},
				{
					ID:     "step-windows-summary",
					Type:   StepAskAI,
					Prompt: "Summarize the Windows host findings. Highlight service failures, event log patterns, disk pressure and suspicious processes.",
				},
			},
		},
	}
}
