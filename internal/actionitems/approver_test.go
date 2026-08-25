package actionitems

import "testing"

func TestIsApproverMatch_UserInList(t *testing.T) {
	got := isApproverMatch("user1", nil, []string{"user1", "user2"}, "")
	if !got {
		t.Error("expected user1 to match")
	}
}

func TestIsApproverMatch_UserNotInListNoRole(t *testing.T) {
	got := isApproverMatch("user3", nil, []string{"user1", "user2"}, "")
	if got {
		t.Error("expected user3 not to match")
	}
}

func TestIsApproverMatch_RoleMatch(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-a", "role-b"}, nil, "role-a")
	if !got {
		t.Error("expected role-a to match")
	}
}

func TestIsApproverMatch_NoRoleConfiguredIgnoresMemberRoles(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-a"}, nil, "")
	if got {
		t.Error("expected no match when no approver role is configured")
	}
}

func TestIsApproverMatch_NeitherUserNorRoleMatches(t *testing.T) {
	got := isApproverMatch("user3", []string{"role-b"}, []string{"user1"}, "role-a")
	if got {
		t.Error("expected no match")
	}
}
