// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxct

import (
	"context"
	"errors"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// The unique id for the builder
const BuilderID = "proxmox.ct"

type Builder struct {
	config        Config
	id            string
	runner        multistep.Runner
	proxmoxClient *proxmox.Client
}

// Builder implements packersdk.Builder
var _ packersdk.Builder = &Builder{}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	return b.config.Prepare(raws...)
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)
	var err error
	b.proxmoxClient, err = common.NewProxmoxClient(b.config.ProxmoxConnect, b.config.PackerDebug)
	if err != nil {
		return nil, err
	}

	// Set up the state
	state.Put("config", &b.config)
	state.Put("proxmoxClient", b.proxmoxClient)
	state.Put("hook", hook)
	state.Put("ui", ui)

	// targetComm := &b.config.Comm
	hostComm := &b.config.Comm

	steps := []multistep.Step{
		&stepCtCreate{},
		&communicator.StepConnect{
			Config:    hostComm,
			Host:      commHost(hostComm.Host()),
			SSHConfig: (*hostComm).SSHConfigFunc(),
		},
		&stepGetCtIpAddr{},
		&stepProvision{},
		&commonsteps.StepCleanupTempKeys{
			Comm: &b.config.Comm,
		},
		&common.StepConvertToTemplate{},
		&common.StepSuccess{}}
	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}
	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("build was cancelled")
	}

	// sb := proxmox.NewSharedBuilder(BuilderID, b.config.Config, preSteps, []multistep.Step{}, &isoVMCreator{})
	artifact := &common.Artifact{
		BuilderID: b.id,
		// templateID:    tplID,
		ProxmoxClient: b.proxmoxClient,
		StateData:     map[string]interface{}{"generated_data": state.Get("generated_data")},
	}

	return artifact, nil
}

// Returns ssh_host or winrm_host (see communicator.Config.Host) config
// parameter when set, otherwise gets the host IP from running VM
func commHost(host string) func(state multistep.StateBag) (string, error) {
	return func(state multistep.StateBag) (string, error) {
		if host == "" {
			return "", errors.New("no host set")
		}
		return host, nil
	}
}
