// FILE 1: artifact.go - CREATE THIS NEW FILE
// Location: /builder/proxmox/ct/artifact.go
// This is a completely new file - copy everything below

package proxmoxct

import (
	"fmt"
	"log"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

// Artifact represents a Proxmox container template
type Artifact struct {
	builderID     string
	templateID    int
	nodeName      string
	templateName  string
	proxmoxClient *proxmox.Client
	stateData     map[string]interface{}
}

// BuilderId returns the builder ID
func (a *Artifact) BuilderId() string {
	return a.builderID
}

// Files returns the files associated with the artifact
// For Proxmox containers, we don't have local files
func (a *Artifact) Files() []string {
	return nil
}

// Id returns a string identifier for the artifact
func (a *Artifact) Id() string {
	return fmt.Sprintf("%s:%d", a.nodeName, a.templateID)
}

// String returns a human-readable string representation
func (a *Artifact) String() string {
	if a.templateName != "" {
		return fmt.Sprintf("Container template '%s' (ID: %d) created on node '%s'",
			a.templateName, a.templateID, a.nodeName)
	}
	return fmt.Sprintf("Container template (ID: %d) created on node '%s'",
		a.templateID, a.nodeName)
}

// State returns state data for the artifact
func (a *Artifact) State(name string) interface{} {
	if a.stateData != nil {
		return a.stateData[name]
	}
	return nil
}

// Destroy removes the container template from Proxmox
func (a *Artifact) Destroy() error {
	log.Printf("[INFO] Destroying container template %d on node %s", a.templateID, a.nodeName)

	// Create a VM reference for the container
	vmRef := proxmox.NewVmRef(a.templateID)
	vmRef.SetNode(a.nodeName)
	vmRef.SetVmType("lxc")

	// First, try to stop the container if it's running
	// This is important for containers that might not be templates yet
	_, stopErr := a.proxmoxClient.StopVm(vmRef)
	if stopErr != nil {
		// It's okay if stop fails - container might already be stopped or be a template
		log.Printf("[WARN] Could not stop container %d (this is normal for templates): %s",
			a.templateID, stopErr)
	}

	// Delete the container/template
	_, err := a.proxmoxClient.DeleteVm(vmRef)
	if err != nil {
		return fmt.Errorf("error destroying container template %d: %s", a.templateID, err)
	}

	log.Printf("[INFO] Successfully destroyed container template %d", a.templateID)
	return nil
}
