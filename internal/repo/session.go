package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type SessionRepository interface {
	Save(ctx context.Context, s domain.Session, ttl time.Duration) error
	Find(ctx context.Context, id string) (domain.Session, error)
	Delete(ctx context.Context, id string) error
	// Hit считает попытку по ключу в окне window и отдаёт число попыток в текущем окне.
	Hit(ctx context.Context, key string, window time.Duration) (int64, error)
	// Put и Take: одноразовая метка с TTL, Take её тут же стирает
	Put(ctx context.Context, key string, ttl time.Duration) error
	Take(ctx context.Context, key string) (bool, error)
}

type sessionRepo struct {
	rdb redis.Cmdable
}

func NewSession(rdb redis.Cmdable) SessionRepository {
	return &sessionRepo{rdb: rdb}
}

type sessionValue struct {
	UserID string      `json:"user_id"`
	Role   domain.Role `json:"role"`
}

func sessionKey(id string) string { return "session:" + id }

func (r *sessionRepo) Save(ctx context.Context, s domain.Session, ttl time.Duration) error {
	v, err := json.Marshal(sessionValue{UserID: s.UserID.String(), Role: s.Role})
	if err != nil {
		return fmt.Errorf("session marshal: %w", err)
	}
	if err := r.rdb.Set(ctx, sessionKey(s.ID), v, ttl).Err(); err != nil {
		return fmt.Errorf("session save: %w", err)
	}
	return nil
}

func (r *sessionRepo) Find(ctx context.Context, id string) (domain.Session, error) {
	raw, err := r.rdb.Get(ctx, sessionKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("session find: %w", err)
	}
	var v sessionValue
	if err = json.Unmarshal(raw, &v); err != nil {
		return domain.Session{}, fmt.Errorf("session decode: %w", err)
	}
	uid, err := uuid.Parse(v.UserID)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session user id: %w", err)
	}
	return domain.Session{ID: id, UserID: uid, Role: v.Role}, nil
}

func (r *sessionRepo) Delete(ctx context.Context, id string) error {
	if err := r.rdb.Del(ctx, sessionKey(id)).Err(); err != nil {
		return fmt.Errorf("session delete: %w", err)
	}
	return nil
}

func (r *sessionRepo) Hit(ctx context.Context, key string, window time.Duration) (int64, error) {
	var n *redis.IntCmd
	_, err := r.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		n = p.Incr(ctx, key)
		// NX: окно отсчитывается от первой попытки, а не продлевается каждой
		p.ExpireNX(ctx, key, window)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("hit %s: %w", key, err)
	}
	return n.Val(), nil
}

func (r *sessionRepo) Put(ctx context.Context, key string, ttl time.Duration) error {
	if err := r.rdb.Set(ctx, key, 1, ttl).Err(); err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	return nil
}

func (r *sessionRepo) Take(ctx context.Context, key string) (bool, error) {
	err := r.rdb.GetDel(ctx, key).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("take %s: %w", key, err)
	}
	return true, nil
}
