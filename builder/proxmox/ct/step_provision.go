package proxmoxct

import (
	"context"
	"log"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepProvision struct {
}

func (s *stepProvision) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	hook := state.Get("hook").(packersdk.Hook)
	ui := state.Get("ui").(packersdk.Ui)
	comm, ok := state.Get("communicator").(packersdk.Communicator)

	if !ok {
		return multistep.ActionHalt
	}

	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	attachComm := &PctCommunicator{
		VmRef:               vmRef,
		WrapperCommunicator: &comm,
	}

	hookData := commonsteps.PopulateProvisionHookData(state)

	// Update state generated_data with complete hookData
	// to make them accessible by post-processors
	state.Put("generated_data", hookData)

	// Provision
	log.Println("Running the provision hook")
	if err := hook.Run(ctx, packersdk.HookProvision, ui, attachComm, hookData); err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *stepProvision) Cleanup(state multistep.StateBag) {
}
