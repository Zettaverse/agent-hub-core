package auth

import (
	"testing"
	"time"
)

func TestIssueAndVerify(t *testing.T) {
	m := NewManager("super-secret", time.Hour)
	token, err := m.Issue("admin", "user-1", "tenant-1", RoleOwner)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "admin" {
		t.Errorf("subject = %q, want admin", claims.Subject)
	}
	if claims.TenantID != "tenant-1" {
		t.Errorf("tenant = %q, want tenant-1", claims.TenantID)
	}
	if claims.Role != RoleOwner {
		t.Errorf("role = %q, want owner", claims.Role)
	}
	if claims.UserID != "user-1" {
		t.Errorf("user id = %q, want user-1", claims.UserID)
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	m := NewManager("right-secret", time.Hour)
	token, _ := m.Issue("admin", "u1", "t1", RoleOwner)
	other := NewManager("wrong-secret", time.Hour)
	if _, err := other.Verify(token); err == nil {
		t.Fatal("expected error verifying with wrong secret")
	}
}

func TestVerifyExpired(t *testing.T) {
	m := NewManager("secret", time.Millisecond)
	token, err := m.Issue("admin", "u1", "t1", RoleOwner)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := m.Verify(token); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestVerifyGarbage(t *testing.T) {
	m := NewManager("secret", time.Hour)
	if _, err := m.Verify("not-a-jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}
