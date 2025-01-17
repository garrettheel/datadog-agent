// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build process
// +build process

package status

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/process/net"
)

// GetSystemProbeStats returns the expvar stats of the system probe
func GetSystemProbeStats(socketPath string) map[string]interface{} {
	probeUtil, err := net.GetRemoteSystemProbeUtil(socketPath)

	if err != nil {
		return map[string]interface{}{
			"Errors": fmt.Sprintf("%v", err),
		}
	}

	systemProbeDetails, err := probeUtil.GetStats()
	if err != nil {
		return map[string]interface{}{
			"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err),
		}
	}

	return systemProbeDetails
}
