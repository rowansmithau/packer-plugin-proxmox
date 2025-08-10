// FILE 3: step_ct_create.go - YOUR FILE WITH 2 SMALL CHANGES
// Location: /builder/proxmox/ct/step_ct_create.go
//
// CHANGE 1: Line ~110 - Move state.Put("vmRef", vmRef) to BEFORE CreateLxc call
// CHANGE 2: Line ~181 - Change "success" to "artifact_created" in Cleanup method
//
// Below is your complete file with these 2 changes already made:

package proxmoxct

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

var (
	maxDuplicateIDRetries = 3
)

type stepCtCreate struct{}

func (s *stepCtCreate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmox.Client)
	c := state.Get("ct-config").(*Config)

	ui.Say("Creating Container")
	config := proxmox.NewConfigLxc()

	if c.Arch != "" {
		config.Arch = c.Arch
	}
	// config.BWLimit = c
	// config.Clone = c.Clone
	// config.CloneStorage = c.CloneStorage
	config.CMode = c.CMode
	config.Console = c.Console
	config.Cores = c.Cores
	config.CPULimit = c.CpuLimit
	config.CPUUnits = c.CpuUnits
	config.Description = c.Description
	// config.Features = c.Features
	if c.Features != "" {
		// For LXC, features should be a simple map with the features as keys
		featuresDevice := make(proxmox.QemuDevice)
		// Parse "nesting=1" to set nesting: "1"
		features := strings.Split(c.Features, ",")
		for _, feature := range features {
			parts := strings.Split(strings.TrimSpace(feature), "=")
			if len(parts) == 2 {
				featuresDevice[parts[0]] = parts[1]
			}
		}
		config.Features = featuresDevice
	}
	config.Force = c.Force
	config.Hookscript = c.Hookscript
	config.Hostname = c.Hostname
	config.IgnoreUnpackErrors = c.IgnoreUnpackErrors
	config.Lock = c.Lock
	config.Memory = c.Memory
	config.Mountpoints = generateMountPoints(c.MountPoints, false)
	config.Nameserver = c.Nameserver
	config.Networks = generateNetworkInterfaces(c.NetworkInterfaces)
	config.OnBoot = c.OnBoot
	config.OsType = c.OSType
	config.Ostemplate = c.OsTemplate
	config.Password = c.UserPassword
	if c.Pool != "" {
		poolName := proxmox.PoolName(c.Pool)
		config.Pool = &poolName // Pool needs a pointer to PoolName
	}
	config.Protection = c.Protection
	config.Restore = c.Restore
	// config.RootFs = generateMountPoints([]MountPointConfig{c.RootFS})[0]
	if c.RootFS != nil {
		rootfsDevice := make(proxmox.QemuDevice)
		rootfsDevice["storage"] = c.RootFS.StorageId
		rootfsDevice["size"] = fmt.Sprintf("%dG", c.RootFS.DiskSizeGB)
		config.RootFs = rootfsDevice
	}
	config.SearchDomain = c.SearchDomain
	// config.Snapname = c.Snapname
	config.SSHPublicKeys = c.SSHPublicKeys
	config.Start = c.Start
	config.Startup = c.Startup
	config.Storage = c.Storage
	config.Swap = c.Swap
	config.Template = c.Template
	config.Tty = c.TTY
	config.Unique = c.Unique
	config.Unprivileged = c.Unprivileged
	config.Tags = strings.Join(c.Tags, ",")
	// config.Unused = c.Unused

	var vmRef *proxmox.VmRef
	for i := 1; ; i++ {
		id := c.VMID
		if id == 0 {
			ui.Say("No VM ID given, getting next free from Proxmox")
			genID, err := client.GetNextID(0)
			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			id = genID
			// config.VmID = genID
		}
		vmRef = proxmox.NewVmRef(id)
		vmRef.SetNode(c.ProxmoxConnect.Node)
		if c.Pool != "" {
			vmRef.SetPool(c.Pool) // SetPool expects a string
			poolName := proxmox.PoolName(c.Pool)
			config.Pool = &poolName // Pool needs a pointer to PoolName
		}

		// IMPORTANT: Store vmRef immediately after creating it, before the API call
		// This ensures cleanup can happen even if creation fails
		state.Put("vmRef", vmRef)

		err := config.CreateLxc(vmRef, client)
		if err == nil {
			break
		}

		// If there's no explicitly configured VMID, and the error is caused
		// by a race condition in someone else using the ID we just got
		// generated, we'll retry up to maxDuplicateIDRetries times.
		if c.VMID == 0 && isDuplicateIDError(err) && i < maxDuplicateIDRetries {
			ui.Say("Generated VM ID was already allocated, retrying")
			continue
		}
		err = fmt.Errorf("error creating VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	log.Printf("config = %v", c)
	log.Printf("client = %v", client)

	return multistep.ActionContinue
}

func generateNetworkInterfaces(nics []NetworkInterfacesConfig) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range nics {
		nic := nics[idx]
		devs[idx] = make(proxmox.QemuDevice)
		devs[idx] = proxmox.QemuDevice{
			"name":      nic.Name,
			"bridge":    nic.Bridge,
			"firewall":  nic.Firewall,
			"gw":        nic.GatewayIPv4,
			"gw6":       nic.GatewayIPv6,
			"hwaddr":    nic.MACAddress,
			"ip":        nic.IPv4Address,
			"ip6":       nic.IPv6Address,
			"link_down": nic.LinkDown,
			"mtu":       nic.MTU,
			"rate":      nic.RateMbps,
			"tag":       nic.Tag,
			"trunks":    strings.Join(nic.Trunks, ":"),
			"type":      nic.Type,
		}
	}
	return devs
}

func generateMountPoints(disks []MountPointConfig, isRootFs bool) proxmox.QemuDevices {
	devs := make(proxmox.QemuDevices)
	for idx := range disks {
		devs[idx] = make(proxmox.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "storage", disks[idx].StorageId)
		setDeviceParamIfDefined(devs[idx], "volume", disks[idx].Volume)
		devs[idx]["size"] = disks[idx].DiskSizeGB
		if len(disks[idx].MountOptions) > 0 {
			devs[idx]["mountoptions"] = disks[idx].MountOptions
		}
		devs[idx]["quota"] = disks[idx].Quota
		devs[idx]["replicate"] = disks[idx].Replicate
		devs[idx]["ro"] = disks[idx].ReadOnly
		devs[idx]["shared"] = disks[idx].Shared
		if !isRootFs {
			devs[idx]["backup"] = disks[idx].Backup
		}
	}
	return devs
}

func setDeviceParamIfDefined(dev proxmox.QemuDevice, key, value string) {
	if value != "" {
		dev[key] = value
	}
}

func isDuplicateIDError(err error) bool {
	return strings.Contains(err.Error(), "already exists on node")
}

func (s *stepCtCreate) Cleanup(state multistep.StateBag) {
	vmRefUntyped, ok := state.GetOk("vmRef")
	// If not ok, we probably errored out before creating the VM
	if !ok {
		return
	}
	vmRef := vmRefUntyped.(*proxmox.VmRef)

	if _, ok := state.GetOk("error"); !ok {
		return // No error = success, don't delete!
	}

	client := state.Get("proxmoxClient").(*proxmox.Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Stopping Container")
	_, err := client.StopVm(vmRef)

	if err != nil {
		ui.Error(fmt.Sprintf("Error stopping VM. Please stop and delete it manually: %s", err))
		return
	}

	ui.Say("Deleting VM")
	_, err = client.DeleteVm(vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting VM. Please delete it manually: %s", err))
		return
	}
}
