package proxmoxct

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/Telmate/proxmox-api-go/proxmox"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/tmp"
)

type PctCommunicator struct {
	VmRef               *proxmox.VmRef
	WrapperCommunicator *packersdk.Communicator
}

func (c *PctCommunicator) Start(ctx context.Context, cmd *packersdk.RemoteCmd) error {
	wrappedCmd := packersdk.RemoteCmd{
		Command: fmt.Sprintf("pct exec %d -- bash -c \"%s\"", c.VmRef.VmId(), cmd.Command),
		Stdin:   cmd.Stdin,
		Stdout:  cmd.Stdout,
		Stderr:  cmd.Stderr,
	}

	// Start the command
	err := c.Execute(ctx, &wrappedCmd, false)

	if err != nil {
		return err
	}

	// Since we rewrapped the command, we have to forward its result to the original passed command
	go func() {
		cmd.SetExited(wrappedCmd.Wait())
	}()

	return nil
}

func (c *PctCommunicator) Upload(dst string, r io.Reader, fi *os.FileInfo) error {
	var src string
	// First upload to remote
	if c.WrapperCommunicator != nil {
		log.Printf("[DEBUG] Uploading %s via the wrapper communicator", dst)
		err := (*c.WrapperCommunicator).Upload(dst, r, fi)
		if err != nil {
			return err
		}
		// In the remote case, we upload
		src = dst
	} else {
		tf, err := tmp.File("packer-pct-push")

		if err != nil {
			return fmt.Errorf("error uploading file to container: %s", err)
		}

		defer os.Remove(tf.Name())

		_, err = io.Copy(tf, r)

		if err != nil {
			return fmt.Errorf("error uploading file to container: %s", err)
		}

		src = tf.Name()
	}

	remoteCmd := packersdk.RemoteCmd{
		Command: fmt.Sprintf("pct push %d %s %s", c.VmRef.VmId(), src, dst),
	}

	// TODO should we delete from the remote here, if we had to do an upload?

	return c.Execute(context.Background(), &remoteCmd, true)
}

func (c *PctCommunicator) UploadDir(dst string, src string, exclude []string) error {
	// find dst -print0 | while IFS= read -r -d '' file; do echo "$file"; done
	var target string
	if c.WrapperCommunicator != nil {
		log.Printf("[DEBUG] Uploading %s from %s via the wrapper communicator", dst, src)
		err := (*c.WrapperCommunicator).UploadDir(dst, src, exclude)
		if err != nil {
			return err
		}
		target = dst
		if src[len(src)-1:] == "/" {
			target += "/" + src
		}
	} else {
		target = src
	}

	remoteCmd := packersdk.RemoteCmd{
		Command: fmt.Sprintf("find %s -print0 | while IFS=' ' read -r -d '' file; do echo \"$file\"; done", target),
	}

	return c.Execute(context.Background(), &remoteCmd, true)

}

func (c *PctCommunicator) Download(src string, w io.Writer) error {
	return fmt.Errorf("Download is not implemented for lxc")

}

func (c *PctCommunicator) DownloadDir(src string, dst string, exclude []string) error {
	return fmt.Errorf("DownloadDir is not implemented for lxc")

}

// Execute runs a command either locally or through the wrapped communicator (i.e. an SSH session)
func (c *PctCommunicator) Execute(ctx context.Context, cmd *packersdk.RemoteCmd, sync bool) error {
	if c.WrapperCommunicator != nil {
		log.Printf("[DEBUG] Starting remote command through wrapper communicator")

		err := (*c.WrapperCommunicator).Start(ctx, cmd)

		log.Println("[DEBUG] Started command remotely")

		if err != nil {
			return err
		}

		// TODO stream outputs
		if sync {
			exitStatus := cmd.Wait()
			if exitStatus != 0 {
				return fmt.Errorf("remote command returned error status %d", exitStatus)
			}
		}

	} else {
		log.Println("Starting command locally")

		return c.executeLocal(cmd)
	}

	return nil
}

func (c *PctCommunicator) CheckInit() error {
	log.Printf("[DEBUG] Debug runlevel exec")
	cmd := packersdk.RemoteCmd{
		Command: "/sbin/runlevel",
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}

	err := c.Execute(context.Background(), &cmd, true)

	exitStatus := cmd.Wait()

	if exitStatus != 0 {
		return fmt.Errorf("got bad exit status from check init: %d", exitStatus)
	}

	if err != nil {
		return err
	}

	return nil
}

func (c *PctCommunicator) executeLocal(cmd *packersdk.RemoteCmd) error {
	log.Printf("[DEBUG] Executing with pct exec in container: %d %s", c.VmRef.VmId(), cmd.Command)

	commandParts := []string{"-c", "pct", "exec", strconv.Itoa(c.VmRef.VmId()), "--", "bash", "-c", "\"" + cmd.Command + "\""}

	localCmd := exec.Command("/bin/sh", commandParts...)
	localCmd.Stdin = cmd.Stdin
	localCmd.Stdout = cmd.Stdout
	localCmd.Stderr = cmd.Stderr

	if err := localCmd.Start(); err != nil {
		return err
	}

	go func() {
		exitStatus := 0
		if err := localCmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitStatus = 1

				// There is no process-independent way to get the REAL
				// exit status so we just try to go deeper.
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitStatus = status.ExitStatus()
				}
			}
		}

		log.Printf(
			"lxc-attach execution exited with '%d': '%s'",
			exitStatus, cmd.Command)
		cmd.SetExited(exitStatus)
	}()

	return nil
}
