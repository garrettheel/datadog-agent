// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dogstatsd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/test/integration/utils"
)

const (
	senderImg string = "datadog/test-origin-detection-sender:master"
)

// testUDSOriginDetection ensures UDS origin detection works, by submitting
// a metric from a container. As we need the origin PID to stay running,
// we can't just `netcat` to the socket, that's why we run a custom python
// script that will stay up after sending packets.
func testUDSOriginDetection(t *testing.T) {
	coreConfig.SetFeatures(t, coreConfig.Docker)

	cfg := map[string]any{}

	// Detect whether we are containerised and set the socket path accordingly
	var socketVolume string
	var composeFile string
	dir := os.ExpandEnv("$SCRATCH_VOLUME_PATH")
	if dir == "" { // Running on the host
		dir = t.TempDir()
		socketVolume = dir
		composeFile = "mount_path.compose"

	} else { // Running in container
		socketVolume = os.ExpandEnv("$SCRATCH_VOLUME_NAME")
		composeFile = "mount_volume.compose"
	}
	socketPath := filepath.Join(dir, "dsd.socket")
	cfg["dogstatsd_socket"] = socketPath
	cfg["dogstatsd_origin_detection"] = true

	confComponent := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule,
		fx.Replace(config.MockParams{Overrides: cfg}),
	))

	// Start DSD
	packetsChannel := make(chan packets.Packets)
	sharedPacketPool := packets.NewPool(32)
	sharedPacketPoolManager := packets.NewPoolManager(sharedPacketPool)
	s, err := listeners.NewUDSListener(packetsChannel, sharedPacketPoolManager, confComponent, nil)
	require.Nil(t, err)

	go s.Listen()
	defer s.Stop()

	compose := &utils.ComposeConf{
		ProjectName: "origin-detection-test",
		FilePath:    fmt.Sprintf("testdata/origin_detection/%s", composeFile),
		Variables:   map[string]string{"socket_dir_path": socketVolume},
	}

	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	containers, err := compose.ListContainers()
	require.Nil(t, err)
	require.True(t, len(containers) > 0)
	containerId := containers[0]
	require.Equal(t, 64, len(containerId))

	t.Logf("Running sender container: %s", containerId)
	stopCmd := exec.Command("docker", "stop", containerId)
	defer stopCmd.Run()

	select {
	case packets := <-packetsChannel:
		packet := packets[0]
		require.NotNil(t, packet)
		require.Equal(t, "custom_counter1:1|c", string(packet.Contents))
		require.Equal(t, fmt.Sprintf("container_id://%s", containerId), packet.Origin)
		sharedPacketPool.Put(packet)
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}
}
