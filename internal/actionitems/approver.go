package actionitems

// ApproverChecker determines whether a Discord user is allowed to complete
// or undo action items, based on a configured set of user IDs and/or a role.
type ApproverChecker struct {
	UserIDs []string
	RoleID  string
}

func (a ApproverChecker) IsApprover(userID string, memberRoleIDs []string) bool {
	for _, id := range a.UserIDs {
		if id == userID {
			return true
		}
	}
	if a.RoleID == "" {
		return false
	}
	for _, role := range memberRoleIDs {
		if role == a.RoleID {
			return true
		}
	}
	return false
}
