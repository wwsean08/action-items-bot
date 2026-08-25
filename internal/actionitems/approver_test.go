package actionitems

import "testing"

func TestApproverChecker_UserIDInList(t *testing.T) {
	checker := ApproverChecker{UserIDs: []string{"user1", "user2"}}

	if !checker.IsApprover("user1", nil) {
		t.Error("expected user1 to be an approver")
	}
}

func TestApproverChecker_UserHasApproverRole(t *testing.T) {
	checker := ApproverChecker{RoleID: "role1"}

	if !checker.IsApprover("someuser", []string{"role1", "otherrole"}) {
		t.Error("expected user with role1 to be an approver")
	}
}

func TestApproverChecker_NeitherMatches(t *testing.T) {
	checker := ApproverChecker{UserIDs: []string{"user1"}, RoleID: "role1"}

	if checker.IsApprover("someuser", []string{"otherrole"}) {
		t.Error("expected user to not be an approver")
	}
}

func TestApproverChecker_EmptyRoleIDNeverMatches(t *testing.T) {
	checker := ApproverChecker{UserIDs: []string{"user1"}}

	if checker.IsApprover("someuser", []string{""}) {
		t.Error("empty RoleID should never match an empty role entry")
	}
}
