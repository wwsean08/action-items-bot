package actionitems

// isApproverMatch reports whether a user is an approver: either directly
// listed in approverUserIDs, or a member of approverRoleID (when configured).
func isApproverMatch(userID string, memberRoleIDs, approverUserIDs []string, approverRoleID string) bool {
	for _, id := range approverUserIDs {
		if id == userID {
			return true
		}
	}
	if approverRoleID == "" {
		return false
	}
	for _, role := range memberRoleIDs {
		if role == approverRoleID {
			return true
		}
	}
	return false
}
