package proxmoxct

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepGetCtIpAddr struct{}

func (s *stepGetCtIpAddr) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	comm, ok := state.Get("communicator").(packersdk.Communicator)
	if !ok {
		state.Put("error", "bad")
		ui.Error("could not retrieve communicator from state")
		return multistep.ActionHalt
	}

	vmRef := state.Get("vmRef").(*proxmox.VmRef)
	var buf bytes.Buffer
	cmd := packersdk.RemoteCmd{
		Command: fmt.Sprintf("lxc-info -n %d -i -H", vmRef.VmId()),
		Stdout:  &buf,
		Stderr:  &buf,
	}
	err := cmd.RunWithUi(ctx, comm, ui)

	if err != nil {
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ip := strings.TrimSuffix(buf.String(), "\n")

	log.Printf("Got output: %s", ip)

	// TODO ensure it's the right format

	state.Put("containerIp", ip)

	return multistep.ActionContinue
}

func (s *stepGetCtIpAddr) Cleanup(state multistep.StateBag) {
}
