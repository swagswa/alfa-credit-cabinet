// Package token выпускает и разбирает JWT сессии (HS256).
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalid = errors.New("invalid token")

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func New(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (i *Issuer) Issue(subject, role string) (string, error) {
	now := i.now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Role:      role,
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	})
	s, err := t.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return s, nil
}

func (i *Issuer) Parse(raw string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return i.secret, nil },
		// без явного списка алгоритмов подписанный "none" или RS-ключом токен прошёл бы разбор
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return &c, nil
}
