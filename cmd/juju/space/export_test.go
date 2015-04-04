// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

func NewCreateCommand(api SpaceAPI) *CreateCommand {
	createCmd := &CreateCommand{}
	createCmd.api = api
	return createCmd
}

func NewRemoveCommand(api SpaceAPI) *RemoveCommand {
	removeCmd := &RemoveCommand{}
	removeCmd.api = api
	return removeCmd
}

func NewUpdateCommand(api SpaceAPI) *UpdateCommand {
	updateCmd := &UpdateCommand{}
	updateCmd.api = api
	return updateCmd
}

func NewRenameCommand(api SpaceAPI) *RenameCommand {
	updateCmd := &RenameCommand{}
	updateCmd.api = api
	return updateCmd
}
