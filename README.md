# Packer Plugin Proxmox
The `Proxmox` multi-component plugin can be used with HashiCorp [Packer](https://www.packer.io)
to create custom images. For the full list of available features for this plugin see [docs](docs).

Per https://github.com/rowansmithau/packer-plugin-proxmox/releases/tag/v1.3.0-container this is a fork 
of https://github.com/chrisbenincasa/packer-plugin-proxmox/tree/ct-create (from 2023) however it has been
brought up to date against the branch which created the official v1.2.3 release.

Provides supported for creating containers in Proxmox via Packer.

A sample container template which uses an updated Rocky Linux (9.6) image:

```
packer {
  required_plugins {
    proxmox = {
      version = "1.3.0"
      source  = "github.com/hashicorp/proxmox"
    }
  }
}

variable "proxmox_url" {
  type = string
  default = "https://YOUR_PROXMOX_URL:8006/api2/json"
}

variable "proxmox_username" {
  type = string
  default = "packer@pve"
}

variable "proxmox_password" {
  type = string
  sensitive = true
  default = "YOUR_PACKER_CREDS
}

source "proxmox-ct" "test" {
  proxmox_url              = var.proxmox_url
  username                 = var.proxmox_username
  password                 = var.proxmox_password
  node                     = "YOUR_PROXMOX_NODE_NAME"
  
  os_template = "local:vztmpl/rockylinux-9-default_20240912_amd64.tar.xz"
  
  rootfs {
    storage_id  = "local-lvm"
    disk_size_gb = 8
  }
  
  network_interfaces {
    name   = "eth0"
    bridge = "vmbr0"
  }
  
  user_password = "12345"
  
  ssh_public_keys = "YOUR_SSH_PUB_KEY"

  features = "nesting=1"  # Enable nesting for Docker/containers inside

  hostname = "jimbob"

  start = true
  # communicator is set to none as most images don't have SSH server built in.
  communicator = "none"
}

build {
  sources = ["source.proxmox-ct.test"]
}
```
