// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// an rmgd chain server.

package btcjson

// SetValidateKeysCmd defines the setvalidatekeys JSON-RPC command.
// This command is not a standard command, it is an extension for operating
// rmgd.
type SetValidateKeysCmd struct {
	PrivKeys []string
}

// NewSetValidateKeysCmd returns a new SetValidateKeysCmd which can
// be used to issue a setvalidatekeys JSON-RPC command.  This command is
// not a standard command. It is an extension for rmgd.
func NewSetValidateKeysCmd(privKeys []string) *SetValidateKeysCmd {
	return &SetValidateKeysCmd{
		PrivKeys: privKeys,
	}
}

func init() {
	// No special flags for commands in this file.
	flags := UsageFlag(0)

	MustRegisterCmd("setvalidatekeys", (*SetValidateKeysCmd)(nil), flags)
}
