package workflow

func DefaultPlaybooks() []Definition {
	return []Definition{
		{
			ID:          "playbook:docker-compose-debug",
			Name:        "Docker Compose Debug",
			Description: "Discovery-first troubleshooting for a Docker Compose service incident.",
			Mode:        ModeApprove,
			Steps: []Step{
				{
					ID:            "step-compose-services-discovery",
					Type:          StepRunDiscovery,
					DiscoveryTool: "discovery:list_compose_services",
				},
				{
					ID:   "step-compose-service-logs",
					Type: StepRunTool,
					Tool: "template:Docker Compose Logs",
					Inputs: map[string]string{
						"Service": "",
					},
				},
				{
					ID:     "step-explain-compose-logs",
					Type:   StepAskAI,
					Prompt: "Explain the compose service logs and identify the most likely failure mode.",
				},
				{
					ID:     "step-suggest-compose-next",
					Type:   StepAskAI,
					Prompt: "Suggest the next safe read-only troubleshooting step for this compose service issue.",
				},
			},
		},
		{
			ID:          "playbook:k8s-pod-triage",
			Name:        "K8s Pod Triage",
			Description: "Namespace and pod focused troubleshooting for Kubernetes incidents.",
			Mode:        ModeApprove,
			Steps: []Step{
				{
					ID:            "step-k8s-namespace-discovery",
					Type:          StepRunDiscovery,
					DiscoveryTool: "discovery:list_k8s_namespaces",
				},
				{
					ID:            "step-k8s-pod-discovery",
					Type:          StepRunDiscovery,
					DiscoveryTool: "discovery:list_k8s_pods",
					Inputs: map[string]string{
						"Namespace": "",
					},
				},
				{
					ID:   "step-k8s-pod-logs",
					Type: StepRunTool,
					Tool: "template:K8s Pod Logs",
					Inputs: map[string]string{
						"Namespace": "",
						"Pod":       "",
					},
				},
				{
					ID:   "step-k8s-describe",
					Type: StepRunTool,
					Tool: "template:K8s Describe Resource",
					Inputs: map[string]string{
						"ResourceType": "pod",
						"Namespace":    "",
						"ResourceName": "",
					},
				},
				{
					ID:     "step-explain-k8s-findings",
					Type:   StepAskAI,
					Prompt: "Summarize the Kubernetes findings and rank the most likely causes of the incident.",
				},
			},
		},
		{
			ID:          "playbook:host-basic-triage",
			Name:        "Host Basic Triage",
			Description: "Initial service, resource and port inspection for a host troubleshooting session.",
			Mode:        ModeAssist,
			Steps: []Step{
				{
					ID:   "step-host-resources",
					Type: StepRunTool,
					Tool: "template:System Resources",
				},
				{
					ID:   "step-host-services",
					Type: StepRunTool,
					Tool: "template:Service Status",
				},
				{
					ID:   "step-host-ports",
					Type: StepRunTool,
					Tool: "template:Port Scanner",
				},
				{
					ID:     "step-explain-host-state",
					Type:   StepAskAI,
					Prompt: "Explain the host state and highlight any resource, service or port anomalies.",
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
	}
}
