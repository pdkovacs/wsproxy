package services

import (
	"slices"
	"strings"
	"wsproxy/test/e2e/app/config"
	"wsproxy/test/e2e/app/security/authr"

	"github.com/rs/zerolog/log"
)

type UsersByGroups map[authr.GroupID][]string

type AuthorizationService interface {
	GetUsers() config.UsersByRoles
	GetGroupsForUser(userID string) []authr.GroupID
	GetPermissionsForGroup(group authr.GroupID) []authr.PermissionID
	GetPermissionsForGroups(group []authr.GroupID) []authr.PermissionID
	UpdateUser(userId string, groups []authr.GroupID)
}

func NewAuthorizationService(options config.Options) authRService {
	return authRService{options.UsersByRoles}
}

type authRService struct {
	// TODO: if this data structure is to serve both the local and the OIDC domain,
	// "usersByGroups" should be more abstract/indirect, like let it be at least a function or something
	usersByGroups config.UsersByRoles
}

func (as *authRService) GetUsers() config.UsersByRoles {
	return as.usersByGroups
}

func (as *authRService) GetGroupsForUser(userID string) []authr.GroupID {
	return getLocalGroupsFor(userID, as.usersByGroups)
}

func (as *authRService) GetPermissionsForGroup(group authr.GroupID) []authr.PermissionID {
	return authr.GetPermissionsForGroup(group)
}

func (as *authRService) GetPermissionsForGroups(groups []authr.GroupID) []authr.PermissionID {
	permissions := []authr.PermissionID{}
	for _, group := range groups {
		permissions = append(permissions, authr.GetPermissionsForGroup(group)...)
	}
	return permissions
}

func (as *authRService) UpdateUser(userId string, groups []authr.GroupID) {
	if as.usersByGroups == nil {
		as.usersByGroups = make(map[string][]string)
	}
	for _, group := range groups {
		as.usersByGroups[string(group)] = append(as.usersByGroups[string(group)], string(userId))
	}
}

func getLocalGroupsFor(userID string, usersByGroups map[string][]string) []authr.GroupID {
	groupNames := []string{}
	for groupName, members := range usersByGroups {
		if slices.Contains(members, string(userID)) {
			groupNames = append(groupNames, groupName)
		}
	}
	log.Debug().Str("user_id", userID).Str("group_names", strings.Join(groupNames, ", ")).Msg("user's group memberships collected")
	return authr.GroupNamesToGroupIDs(groupNames)
}
