// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,MountPointConfig,NetworkInterfacesConfig

package proxmoxct

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"time"

	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Comm                communicator.Config                `mapstructure:",squash"`
	ProxmoxConnect      proxmoxcommon.ProxmoxConnectConfig `mapstructure:",squash"`

	// Required
	OsTemplate string `mapstructure:"os_template"`
	VMID       int    `mapstructure:"vm_id"`

	// Optional
	Arch               string                    `mapstructure:"arch"`
	BwLimit            int                       `mapstructure:"bw_limit"`
	CMode              string                    `mapstructure:"cmode"`
	Console            bool                      `mapstructure:"console"`
	Cores              int                       `mapstructure:"cores"`
	CpuLimit           int                       `mapstructure:"cpu_limit"`
	CpuUnits           int                       `mapstructure:"cpu_units"`
	Debug              bool                      `mapstructure:"debug"`
	Description        string                    `mapstructure:"description"`
	Features           string                    `mapstructure:"features"`
	Force              bool                      `mapstructure:"force"`
	Hookscript         string                    `mapstructure:"hookscript"`
	Hostname           string                    `mapstructure:"hostname"`
	IgnoreUnpackErrors bool                      `mapstructure:"ignore_unpack_errors"`
	Lock               string                    `mapstructure:"lock"`
	Memory             int                       `mapstructure:"memory"`
	MountPoints        []MountPointConfig        `mapstructure:"mount_points"`
	Nameserver         string                    `mapstructure:"nameserver"`
	NetworkInterfaces  []NetworkInterfacesConfig `mapstructure:"network_interfaces"`
	OnBoot             bool                      `mapstructure:"on_boot"`
	OSType             string                    `mapstructure:"os_type"`
	UserPassword       string                    `mapstructure:"user_password"`
	Pool               string                    `mapstructure:"pool"`
	Protection         bool                      `mapstructure:"protection"`
	Restore            bool                      `mapstructure:"restore"`
	RootFS             *MountPointConfig         `mapstructure:"rootfs"`
	SearchDomain       string                    `mapstructure:"search_domain"`
	SSHPublicKeys      string                    `mapstructure:"ssh_public_keys"`
	Start              bool                      `mapstructure:"start"`
	Startup            string                    `mapstructure:"startup"`
	Storage            string                    `mapstructure:"storage"`
	Swap               int                       `mapstructure:"swap"`
	Tags               []string                  `mapstructure:"tags"`
	Template           bool                      `mapstructure:"template"`
	Timezone           string                    `mapstructure:"timezone"`
	TTY                int                       `mapstructure:"tty"`
	Unique             bool                      `mapstructure:"unique"`
	Unprivileged       bool                      `mapstructure:"unprivileged"`
	UnusedVolumes      []string                  `mapstructure:"unused_volumes"`

	Ctx interpolate.Context `mapstructure-to-hcl2:",skip"`
}

type MountPointConfig struct {
	StorageId    string                 `mapstructure:"storage_id"`
	Volume       string                 `mapstructure:"volume"`
	Path         string                 `mapstructure:"path"`
	ACL          bool                   `mapstructure:"acl"`
	Backup       bool                   `mapstructure:"backup"`
	MountOptions map[string]interface{} `mapstructure:"mount_options"`
	Quota        bool                   `mapstructure:"quota"`
	Replicate    bool                   `mapstructure:"replicate"`
	ReadOnly     bool                   `mapstructure:"readonly"`
	Shared       bool                   `mapstructure:"shared"`
	DiskSizeGB   int                    `mapstructure:"disk_size_gb"`
}

type NetworkInterfacesConfig struct {
	Name        string   `mapstructure:"name"`
	Bridge      string   `mapstructure:"bridge"`
	Firewall    bool     `mapstructure:"firewall"`
	GatewayIPv4 string   `mapstructure:"gateway_ipv4"`
	GatewayIPv6 string   `mapstructure:"gateway_ipv6"`
	MACAddress  string   `mapstructure:"mac_address"`
	IPv4Address string   `mapstructure:"ipv4_address"`
	IPv6Address string   `mapstructure:"ipv6_address"`
	LinkDown    bool     `mapstructure:"link_down"`
	MTU         int      `mapstructure:"mtu"`
	RateMbps    int      `mapstructure:"rate_mbps"`
	Tag         int      `mapstructure:"tag"`
	Trunks      []string `mapstructure:"trunks"`
	Type        string   `mapstructure:"type"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, []string, error) {
	var md mapstructure.Metadata
	err := config.Decode(c, &config.DecodeOpts{
		Metadata:           &md,
		Interpolate:        true,
		InterpolateContext: &c.Ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"boot_command",
			},
		},
	}, raws...)
	if err != nil {
		return nil, nil, err
	}

	var errs *packersdk.MultiError
	var warnings []string

	proxmoxConnectConf := c.ProxmoxConnect

	packersdk.LogSecretFilter.Set(proxmoxConnectConf.Password)

	// Defaults
	if proxmoxConnectConf.ProxmoxURLRaw == "" {
		proxmoxConnectConf.ProxmoxURLRaw = os.Getenv("PROXMOX_URL")
	}
	if proxmoxConnectConf.Username == "" {
		proxmoxConnectConf.Username = os.Getenv("PROXMOX_USERNAME")
	}
	if proxmoxConnectConf.Password == "" {
		proxmoxConnectConf.Password = os.Getenv("PROXMOX_PASSWORD")
	}
	if proxmoxConnectConf.Token == "" {
		proxmoxConnectConf.Token = os.Getenv("PROXMOX_TOKEN")
	}
	if proxmoxConnectConf.TaskTimeout == 0 {
		c.ProxmoxConnect.TaskTimeout = 60 * time.Second
	}
	c.ProxmoxConnect = proxmoxConnectConf

	if c.ProxmoxConnect.ProxmoxURL, err = url.Parse(c.ProxmoxConnect.ProxmoxURLRaw); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse proxmox_url: %s", err))
	}

	if c.ProxmoxConnect.TaskTimeout == 0 {
		c.ProxmoxConnect.TaskTimeout = 60 * time.Second
	}

	// Defaults from https://pve.proxmox.com/pve-docs/api-viewer/#/nodes/{node}/lxc
	if c.Arch == "" {
		c.Arch = "amd64"
	}
	if c.Memory < 16 {
		log.Printf("Memory %d is too small, using default: 512", c.Memory)
		c.Memory = 512
	}
	if c.Cores < 1 {
		log.Printf("Number of cores %d is too small, using default: 1", c.Cores)
		c.Cores = 1
	}
	if c.Swap < 0 {
		log.Printf("Swap size %d is too small, using default: 512", c.Swap)
		c.Swap = 512
	}
	if c.CMode == "" || (c.CMode != "shell" && c.CMode != "tty" && c.CMode != "console") {
		log.Printf("Invalid console mode specified (%s), using default: tty", c.CMode)
		c.CMode = "tty"
	}
	if c.TTY <= 0 || c.TTY > 6 {
		log.Printf("Invalid TTY size specified (%d), using default: 2", c.TTY)
		c.TTY = 2
	}
	if c.CpuUnits < 8 {
		c.CpuUnits = 1024
	}
	if c.Hostname == "" {
		// Default to packer-[time-ordered-uuid]
		c.Hostname = fmt.Sprintf("packer-%s", uuid.TimeOrderedUUID())
	}

	// Validation
	errs = packersdk.MultiErrorAppend(errs, c.Comm.Prepare(&c.Ctx)...)

	// Required configurations that will display errors if not set
	if c.ProxmoxConnect.Username == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("username must be specified"))
	}
	if c.ProxmoxConnect.Password == "" && c.ProxmoxConnect.Token == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("password or token must be specified"))
	}
	if c.ProxmoxConnect.ProxmoxURLRaw == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("proxmox_url must be specified"))
	}
	if c.ProxmoxConnect.ProxmoxURL, err = url.Parse(c.ProxmoxConnect.ProxmoxURLRaw); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("could not parse proxmox_url: %s", err))
	}
	if c.ProxmoxConnect.Node == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("node must be specified"))
	}

	if c.OsTemplate == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("os_template must be specified"))
	}

	// Technically Proxmox VMIDs are unsigned 32bit integers, but are limited to
	// the range 100-999999999. Source:
	// https://pve-devel.pve.proxmox.narkive.com/Pa6mH1OP/avoiding-vmid-reuse#post8
	if c.VMID != 0 && (c.VMID < 100 || c.VMID > 999999999) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("vm_id must be in range 100-999999999"))
	}

	// Verify VM Name and Template Name are a valid DNS Names
	re := regexp.MustCompile(`^(?:(?:(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]*[a-zA-Z0-9])?)\.)*(?:[A-Za-z0-9](?:[A-Za-z0-9\-]*[A-Za-z0-9])?))$`)
	if !re.MatchString(c.Hostname) {
		errs = packersdk.MultiErrorAppend(errs, errors.New("vm_name must be a valid DNS name"))
	}

	if c.RootFS == nil {
		errs = packersdk.MultiErrorAppend(errs, errors.New("rootfs block must be specified"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, warnings, errs
	}
	return nil, warnings, nil
}
