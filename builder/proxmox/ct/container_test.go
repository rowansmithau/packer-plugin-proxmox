// FILE: container_test.go - FIXED VERSION
// The issue was using "User" instead of "SSHUsername" in communicator.Config

package proxmoxct

import (
	"testing"

	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
)

func TestConfig_Prepare_Minimal(t *testing.T) {
	cfg := &Config{
		Comm: communicator.Config{
			Type: "none", // No SSH needed for basic container tests
		},
		Hostname:     "test-container",
		OsTemplate:   "local:vztmpl/debian-11-standard_11.0-1_amd64.tar.gz",
		UserPassword: "testpass",
		Storage:      "local-lvm",
		Memory:       512,
		Cores:        1,
		VMID:         1001,
		ProxmoxConnect: common.Config{
			Node:          "pve-node",
			ProxmoxURLRaw: "https://proxmox.example.com:8006/api2/json",
			Username:      "root@pam",
			Password:      "dummy",
		},
		RootFS: &MountPointConfig{
			StorageId:  "local-lvm",
			DiskSizeGB: 8,
		},
	}
	warnings, errors, err := cfg.Prepare(nil)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if len(errors) > 0 {
		t.Errorf("Prepare returned errors: %v", errors)
	}
	if len(warnings) > 0 {
		t.Logf("Prepare returned warnings: %v", warnings)
	}
}

func TestBuilder_ConfigSpec(t *testing.T) {
	b := &Builder{}
	if b.ConfigSpec() == nil {
		t.Error("ConfigSpec should not return nil")
	}
}

func TestBuilder_Prepare_Invalid(t *testing.T) {
	b := &Builder{}
	_, errors, err := b.Prepare(map[string]interface{}{"invalid": true})
	if err == nil && len(errors) == 0 {
		t.Error("Expected error or errors for invalid config")
	}
}

func TestNewProxmoxClient_InvalidConfig(t *testing.T) {
	_, err := newProxmoxClient(common.Config{})
	if err == nil {
		t.Error("Expected error for invalid config")
	}
}
