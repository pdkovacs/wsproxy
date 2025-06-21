package authr

import (
	"errors"
	"fmt"
)

type PermissionID string

const (
	CREATE_ICON     PermissionID = "CREATE_ICON"
	UPDATE_ICON     PermissionID = "UPDATE_ICON"
	ADD_ICONFILE    PermissionID = "ADD_ICONFILE"
	REMOVE_ICONFILE PermissionID = "REMOVE_ICONFILE"
	REMOVE_ICON     PermissionID = "REMOVE_ICON"
	ADD_TAG         PermissionID = "ADD_TAG"
	REMOVE_TAG      PermissionID = "REMOVE_TAG"
)

func GetPrivilegeString(id PermissionID) string {
	return string(id)
}

type GroupID string

const (
	ICON_EDITOR GroupID = "ICON_EDITOR"
)

var permissionsByGroup = map[GroupID][]PermissionID{
	ICON_EDITOR: {
		CREATE_ICON,
		UPDATE_ICON,
		ADD_ICONFILE,
		REMOVE_ICONFILE,
		REMOVE_ICON,
		ADD_TAG,
		REMOVE_TAG,
	},
}

func GetPermissionsForGroup(group GroupID) []PermissionID {
	return permissionsByGroup[group]
}

var ErrPermission = errors.New("permission error")

func HasRequiredPermissions(userInfo UserInfo, requiredPermissions []PermissionID) error {
	for _, reqPerm := range requiredPermissions {
		found := false
		for _, uPerm := range userInfo.Permissions {
			if reqPerm == uPerm {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("not all of %v is included in %v granted to %v, %w", requiredPermissions, userInfo.Permissions, userInfo.UserId, ErrPermission)
		}
	}
	return nil
}
