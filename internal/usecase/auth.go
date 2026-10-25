package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

const sessionIDBytes = 32

// alfaStateTTL сколько ждём клиента со страницы согласия Альфа ID
const alfaStateTTL = 5 * time.Minute

// dummyHash сравнивается, когда логина нет: иначе по времени ответа видно,
// существует ли пользователь (bcrypt занимает десятки миллисекунд).
const dummyHash = "$2a$10$04Zt0uJ6BnjrJWX6bIgz5eeUSNrpBbPJiaoksCu7qAAFO8gfBxXvq"

type AuthUseCase interface {
	// Login отдаёт пользователя с его компаниями и токен для cookie.
	Login(ctx context.Context, login, password, ip string) (domain.Me, string, error)
	Logout(ctx context.Context, sessionID string) error
	// Resolve находит сессию по токену из cookie. Любой негодный токен это ErrUnauthorized.
	Resolve(ctx context.Context, raw string) (domain.Session, error)
	Me(ctx context.Context, userID uuid.UUID) (domain.Me, error)
	// AlfaIDStart отдаёт state для страницы согласия, AlfaIDLogin по нему и коду
	// заводит сессию. Протухший или чужой state и код это ErrUnauthorized,
	// клиента Альфа ID нет в кабинете: ErrNotFound.
	AlfaIDStart(ctx context.Context) (string, error)
	AlfaIDLogin(ctx context.Context, state, code, ip string) (string, error)
}

// AlfaID провайдер входа, на стенде симулятор в mocks.
type AlfaID interface {
	Profile(ctx context.Context, code string) (domain.AlfaProfile, error)
}

type LoginLimit struct {
	Attempts int64
	Window   time.Duration
}

type authUseCase struct {
	users    repo.UserRepository
	sessions repo.SessionRepository
	alfa     AlfaID
	tokens   *token.Issuer
	limit    LoginLimit
}

func NewAuth(
	users repo.UserRepository,
	sessions repo.SessionRepository,
	alfa AlfaID,
	tokens *token.Issuer,
	limit LoginLimit,
) AuthUseCase {
	return &authUseCase{users: users, sessions: sessions, alfa: alfa, tokens: tokens, limit: limit}
}

// attempt считает попытку входа с адреса, общий счётчик для пароля и Альфа ID
func (u *authUseCase) attempt(ctx context.Context, ip string) error {
	n, err := u.sessions.Hit(ctx, "login_attempts:"+ip, u.limit.Window)
	if err != nil {
		return err
	}
	if n > u.limit.Attempts {
		return domain.ErrTooManyRequests
	}
	return nil
}

func (u *authUseCase) Login(ctx context.Context, login, password, ip string) (domain.Me, string, error) {
	if err := u.attempt(ctx, ip); err != nil {
		return domain.Me{}, "", err
	}

	user, hash, err := u.users.Credentials(ctx, login)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return domain.Me{}, "", domain.ErrUnauthorized
	case err != nil:
		return domain.Me{}, "", err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return domain.Me{}, "", domain.ErrUnauthorized
	}

	me, err := u.Me(ctx, user.ID)
	if err != nil {
		return domain.Me{}, "", err
	}

	raw, err := u.newSession(ctx, user)
	if err != nil {
		return domain.Me{}, "", err
	}
	return me, raw, nil
}

func (u *authUseCase) AlfaIDStart(ctx context.Context) (string, error) {
	state := rand.Text()
	return state, u.sessions.Put(ctx, "alfa_id_state:"+state, alfaStateTTL)
}

func (u *authUseCase) AlfaIDLogin(ctx context.Context, state, code, ip string) (string, error) {
	if err := u.attempt(ctx, ip); err != nil {
		return "", err
	}
	ok, err := u.sessions.Take(ctx, "alfa_id_state:"+state)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", domain.ErrUnauthorized
	}
	p, err := u.alfa.Profile(ctx, code)
	if err != nil {
		return "", err
	}
	user, _, err := u.users.Credentials(ctx, p.Login)
	if err != nil {
		return "", err
	}
	// контакты у банка свежее наших: клиент мог сменить телефон в Альфа ID
	if err := u.users.SetContacts(ctx, user.ID, p.Phone, p.Email); err != nil {
		return "", err
	}
	return u.newSession(ctx, user)
}

// newSession заводит сессию в хранилище и отдаёт подписанный токен с её ID.
func (u *authUseCase) newSession(ctx context.Context, user domain.User) (string, error) {
	b := make([]byte, sessionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	s := domain.Session{ID: hex.EncodeToString(b), UserID: user.ID, Role: user.Role}
	if err := u.sessions.Save(ctx, s, u.tokens.TTL()); err != nil {
		return "", err
	}
	return u.tokens.Issue(s.ID)
}

func (u *authUseCase) Logout(ctx context.Context, sessionID string) error {
	return u.sessions.Delete(ctx, sessionID)
}

func (u *authUseCase) Resolve(ctx context.Context, raw string) (domain.Session, error) {
	sid, err := u.tokens.Parse(raw)
	if err != nil {
		return domain.Session{}, domain.ErrUnauthorized
	}
	s, err := u.sessions.Find(ctx, sid)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Session{}, domain.ErrUnauthorized
	}
	return s, err
}

func (u *authUseCase) Me(ctx context.Context, userID uuid.UUID) (domain.Me, error) {
	user, err := u.users.ByID(ctx, userID)
	if err != nil {
		return domain.Me{}, err
	}
	ms, err := u.users.Memberships(ctx, userID)
	if err != nil {
		return domain.Me{}, err
	}
	return domain.Me{User: user, Memberships: ms}, nil
}
