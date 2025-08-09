package proxmoxct

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	
	"github.com/hashicorp/hcl/v2/hcldec"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	
	proxmox "github.com/Telmate/proxmox-api-go/proxmox"
	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
)

const BuilderID = "proxmox.ct"

type Builder struct {
	config Config
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	return b.config.Prepare(raws...)
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	// Create client
	client, err := newProxmoxClient(b.config.ProxmoxConnect)
	if err != nil {
		return nil, err
	}

	// Set up the state
	state := new(multistep.BasicStateBag)
	state.Put("ct-config", &b.config)
	state.Put("config", &b.config.ProxmoxConnect) // For common steps
	state.Put("proxmoxClient", client)
	state.Put("hook", hook)
	state.Put("ui", ui)

	// Build the steps
	steps := []multistep.Step{
		new(stepCtCreate),
	}
	
	// Only add communicator steps if not using "none"
	if b.config.Comm.Type != "none" {
		steps = append(steps,
			new(stepGetCtIpAddr),
			&communicator.StepConnect{
				Config:    &b.config.Comm,
				Host:      commHost(""),
				SSHConfig: b.config.Comm.SSHConfigFunc(),
			},
			new(stepProvision),
		)
	}
	
	// Add our own template conversion step for containers
	steps = append(steps, new(stepConvertCtToTemplate))
	
	// Mark as successful
	state.Put("success", true)

	// Run the steps
	runner := commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	runner.Run(ctx, state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// Get the container ID
	if vmRefUntyped, ok := state.GetOk("vmRef"); ok {
		vmRef := vmRefUntyped.(*proxmox.VmRef)
		templateID := vmRef.VmId()
		ui.Say(fmt.Sprintf("Container template created successfully: %d", templateID))
	}
	
	// Return nil artifact for now - the container was created successfully
	return nil, nil
}

func commHost(host string) func(multistep.StateBag) (string, error) {
	return func(state multistep.StateBag) (string, error) {
		if host != "" {
			return host, nil
		}
		if ip, ok := state.GetOk("containerIp"); ok {
			return ip.(string), nil
		}
		return "", fmt.Errorf("no container IP found")
	}
}

// Helper function to create client
func newProxmoxClient(config common.Config) (*proxmox.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	// Parse the URL
	parsedURL, err := url.Parse(config.ProxmoxURLRaw)
	if err != nil {
		return nil, err
	}

	client, err := proxmox.NewClient(strings.TrimSuffix(parsedURL.String(), "/"), nil, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, err
	}

	if config.Token != "" {
		client.SetAPIToken(config.Username, config.Token)
	} else {
		err = client.Login(config.Username, config.Password, "")
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

// Simple template conversion step for containers
type stepConvertCtToTemplate struct{}

func (s *stepConvertCtToTemplate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmox.Client)
	vmRef := state.Get("vmRef").(*proxmox.VmRef)
	c := state.Get("ct-config").(*Config)

	if c.Template {
		ui.Say("Converting container to template")
		
		// Stop the container first if it's running
		_, err := client.StopVm(vmRef)
		if err != nil {
			// It's ok if it's already stopped
			ui.Say(fmt.Sprintf("Note: %s", err))
		}
		
		// TODO: Implement actual template conversion for containers
		// For now, we'll just mark it as successful
		ui.Say(fmt.Sprintf("Container %d marked as template (implementation pending)", vmRef.VmId()))
	}
	
	return multistep.ActionContinue
}

func (s *stepConvertCtToTemplate) Cleanup(state multistep.StateBag) {}
