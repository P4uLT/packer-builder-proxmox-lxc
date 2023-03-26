// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lxctemplate

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/communicator/ssh"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
)

// StepSshKeyPair executes the business logic for setting the SSH key pair in
// the specified communicator.Config.
type StepSshKeyPair struct {
	Debug        bool
	DebugKeyPath string
}

func (s *StepSshKeyPair) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	c := state.Get("config").(*Config)

	if c.Comm.SSHPassword != "" {
		ui.Say("using password for the communicator...")
		return multistep.ActionContinue
	}

	if c.Comm.SSHPrivateKeyFile != "" {
		ui.Say("using existing SSH private key for the communicator...")
		privateKeyBytes, err := c.Comm.ReadSSHPrivateKeyFile()
		if err != nil {
			state.Put("error", err)
			return multistep.ActionHalt
		}

		kp, err := ssh.KeyPairFromPrivateKey(ssh.FromPrivateKeyConfig{
			RawPrivateKeyPemBlock: privateKeyBytes,
			Comment:               fmt.Sprintf("packer_%s", uuid.TimeOrderedUUID()),
		})
		if err != nil {
			state.Put("error", err)
			return multistep.ActionHalt
		}

		c.Comm.SSHPrivateKey = privateKeyBytes
		c.Comm.SSHKeyPairName = kp.Comment
		c.Comm.SSHTemporaryKeyPairName = kp.Comment
		c.Comm.SSHPublicKey = kp.PublicKeyAuthorizedKeysLine

		return multistep.ActionContinue
	}

	if c.Comm.SSHAgentAuth {
		ui.Say("using local SSH Agent to authenticate connections for the communicator...")
		return multistep.ActionContinue
	}

	ui.Say("creating ephemeral key pair for SSH communicator...")

	kp, err := ssh.NewKeyPair(ssh.CreateKeyPairConfig{
		Comment: fmt.Sprintf("packer_%s", uuid.TimeOrderedUUID()),
	})
	if err != nil {
		state.Put("error", fmt.Errorf("error creating temporary keypair: %s", err))
		return multistep.ActionHalt
	}

	c.Comm.SSHKeyPairName = kp.Comment
	c.Comm.SSHTemporaryKeyPairName = kp.Comment
	c.Comm.SSHPrivateKey = kp.PrivateKeyPemBlock
	c.Comm.SSHPublicKey = kp.PublicKeyAuthorizedKeysLine
	c.Comm.SSHClearAuthorizedKeys = true

	ui.Say("created ephemeral SSH key pair for communicator")

	// If we're in debug mode, output the private key to the working
	// directory.
	if s.Debug {
		ui.Message(fmt.Sprintf("saving communicator private key for debug purposes: %s", s.DebugKeyPath))
		f, err := os.OpenFile(s.DebugKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			state.Put("error", fmt.Errorf("error saving debug key: %s", err))
			return multistep.ActionHalt
		}
		defer f.Close()

		// Write the key out
		if _, err := f.Write(kp.PrivateKeyPemBlock); err != nil {
			state.Put("error", fmt.Errorf("error saving debug key: %s", err))
			return multistep.ActionHalt
		}
	}

	return multistep.ActionContinue
}

func (s *StepSshKeyPair) Cleanup(state multistep.StateBag) {
	if s.Debug {
		if err := os.Remove(s.DebugKeyPath); err != nil {
			ui := state.Get("ui").(packersdk.Ui)
			ui.Error(fmt.Sprintf(
				"Error removing debug key '%s': %s", s.DebugKeyPath, err))
		}
	}
}
