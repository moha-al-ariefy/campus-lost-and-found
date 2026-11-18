package backend

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type IdentityProvider interface {
	Authenticate(ctx context.Context, login, password string) (*Account, error)
}

type PrototypeIdentityProvider struct{ DB *sql.DB }

func (p PrototypeIdentityProvider) Authenticate(ctx context.Context, login, password string) (*Account, error) {
	var account Account
	var points, salt, expected string
	err := p.DB.QueryRowContext(ctx, `SELECT id,name,role,holding_points,salt,password_hash
		FROM accounts WHERE login=?`, strings.ToLower(strings.TrimSpace(login))).
		Scan(&account.ID, &account.Name, &account.Role, &points, &salt, &expected)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	found := err == nil
	if !found {
		salt, expected = strings.Repeat("0", 32), strings.Repeat("0", 128)
	}
	computed, err := derivePassword(password, salt)
	if err != nil {
		return nil, err
	}
	match := subtle.ConstantTimeCompare([]byte(hex.EncodeToString(computed)), []byte(expected))
	if !found || match != 1 {
		return nil, ErrInvalidCredentials
	}
	if err := json.Unmarshal([]byte(points), &account.HoldingPoints); err != nil {
		return nil, err
	}
	return &account, nil
}
