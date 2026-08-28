// Package auth implements JWT issuance/verification, HTTP middleware, and
// role-based access control.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT claim set used by the hub.
type Claims struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
	UserID   string `json:"uid"`
	jwt.RegisteredClaims
}

// Manager issues and verifies HS256 JWTs.
type Manager struct {
	secret []byte
	expiry time.Duration
	now    func() time.Time
}

// NewManager returns a Manager with the given HMAC secret and token lifetime.
func NewManager(secret string, expiry time.Duration) *Manager {
	return &Manager{
		secret: []byte(secret),
		expiry: expiry,
		now:    time.Now,
	}
}

var (
	ErrInvalidToken = errors.New("auth: invalid token")
	ErrExpiredToken = errors.New("auth: expired token")
)

// Issue creates a signed token for the given subject (username), tenant and
// role.
func (m *Manager) Issue(subject, userID, tenantID, role string) (string, error) {
	now := m.now()
	claims := Claims{
		TenantID: tenantID,
		Role:     role,
		UserID:   userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiry)),
			Issuer:    "agent-hub-core",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Verify parses and validates a token, returning its claims.
func (m *Manager) Verify(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.TenantID == "" || claims.Role == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Context keys for values populated by the middleware.
type ctxKey int

const (
	ctxClaims ctxKey = iota
)

// WithClaims stores claims in the context.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxClaims, c)
}

// ClaimsFrom returns the claims stored in the context, if any.
func ClaimsFrom(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxClaims).(*Claims)
	return c, ok
}

// TenantIDFrom returns the tenant id stored in the context.
func TenantIDFrom(ctx context.Context) string {
	if c, ok := ClaimsFrom(ctx); ok {
		return c.TenantID
	}
	return ""
}

// RoleFrom returns the role stored in the context.
func RoleFrom(ctx context.Context) string {
	if c, ok := ClaimsFrom(ctx); ok {
		return c.Role
	}
	return ""
}
