// Package token выпускает и разбирает JWT сессии (HS256). Токен несёт только
// id серверной сессии и срок, всё остальное лежит в самой сессии.
package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalid = errors.New("invalid token")

type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func New(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (i *Issuer) TTL() time.Duration { return i.ttl }

func (i *Issuer) Issue(sessionID string) (string, error) {
	now := i.now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sessionID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	})
	s, err := t.SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return s, nil
}

// Parse отдаёт id сессии из токена.
func (i *Issuer) Parse(raw string) (string, error) {
	var c jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return i.secret, nil },
		// без явного списка алгоритмов подписанный "none" или RS-ключом токен прошёл бы разбор
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(i.now),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return c.Subject, nil
}
