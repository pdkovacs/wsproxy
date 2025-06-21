package authr

func GroupNamesToGroupIDs(groups []string) []GroupID {
	memberIn := []GroupID{}
	for _, group := range groups {
		memberIn = append(memberIn, GroupID(group))
	}
	return memberIn
}

type UserInfo struct {
	UserId      string         `json:"userID"`
	Groups      []GroupID      `json:"groups"`
	Permissions []PermissionID `json:"permissions"`
	DisplayName string         `json:"displayName"`
}
