package services

import (
	"wsproxy/test/e2e/app/security/authr"
)

func NewUserService(authorizationService AuthorizationService) UserService {
	return UserService{
		AuthorizationService: authorizationService,
	}
}

type UserService struct {
	AuthorizationService AuthorizationService
}

func (us *UserService) getPermissionsForUser(userId string) []authr.PermissionID {
	userPermissions := []authr.PermissionID{}

	memberIn := us.AuthorizationService.GetGroupsForUser(userId)
	for _, group := range memberIn {
		userPermissions = append(userPermissions, authr.GetPermissionsForGroup(group)...)
	}

	return userPermissions
}

func (us *UserService) getDisplayName(userId string) string {
	return userId
}

func (us *UserService) GetUserInfo(userId string) authr.UserInfo {
	memberIn := us.AuthorizationService.GetGroupsForUser(userId)
	return authr.UserInfo{
		UserId:      userId,
		Groups:      memberIn,
		Permissions: us.getPermissionsForUser(userId),
		DisplayName: us.getDisplayName(userId),
	}
}

func (us *UserService) UpdateUserInfo(userId string, memberIn []authr.GroupID) {
	us.AuthorizationService.UpdateUser(userId, memberIn)
}
