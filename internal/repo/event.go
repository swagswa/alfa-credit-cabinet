package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
)

const (
	eventChannel = "applications"
	relisten     = 2 * time.Second
	// столько событий подписчик может не забрать, дальше считаем его отставшим
	subBuffer = 32
)

// EventRepository держит один LISTEN на процесс и раздаёт уведомления
// подписчикам. Как svc.Service слушает, пока жив процесс.
type EventRepository interface {
	svc.Service
	// Subscribe отдаёт канал событий, прошедших keep. Канал закрывается, если
	// подписчик отстал или слушатель переподключался: пропущенное не вернуть,
	// клиенту остаётся переподключиться и перечитать. cancel обязателен.
	Subscribe(keep func(domain.Event) bool) (events <-chan domain.Event, cancel func())
}

type subscriber struct {
	ch   chan domain.Event
	keep func(domain.Event) bool
}

type eventRepo struct {
	log *zap.Logger
	db  *postgres.Client

	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

func NewEvent(log *zap.Logger, db *postgres.Client) EventRepository {
	return &eventRepo{log: log, db: db, subs: map[*subscriber]struct{}{}}
}

func (r *eventRepo) Name() string               { return "events" }
func (r *eventRepo) Init(context.Context) error { return nil }
func (r *eventRepo) Stop(context.Context) error { return nil }

func (r *eventRepo) Run(ctx context.Context) error {
	for {
		err := r.listen(ctx)
		r.dropAll()
		if ctx.Err() == nil {
			r.log.Warn("listen "+eventChannel, zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(relisten):
		}
	}
}

func (r *eventRepo) listen(ctx context.Context) error {
	pc, err := r.db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	// с подпиской соединение в пул не возвращаем, закрываем сами
	conn := pc.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	if _, err := conn.Exec(ctx, "LISTEN "+eventChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait: %w", err)
		}
		var ev domain.Event
		if err := json.Unmarshal([]byte(n.Payload), &ev); err != nil {
			r.log.Error("event payload", zap.String("payload", n.Payload), zap.Error(err))
			continue
		}
		r.publish(ev)
	}
}

func (r *eventRepo) Subscribe(keep func(domain.Event) bool) (events <-chan domain.Event, cancel func()) {
	s := &subscriber{ch: make(chan domain.Event, subBuffer), keep: keep}
	r.mu.Lock()
	r.subs[s] = struct{}{}
	r.mu.Unlock()
	return s.ch, func() { r.drop(s) }
}

func (r *eventRepo) publish(ev domain.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs {
		if !s.keep(ev) {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			// медленный клиент не должен тормозить остальных
			r.dropLocked(s)
		}
	}
}

func (r *eventRepo) drop(s *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dropLocked(s)
}

func (r *eventRepo) dropAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.subs {
		r.dropLocked(s)
	}
}

// dropLocked можно звать повторно: cancel после закрытия по отставанию не паникует.
func (r *eventRepo) dropLocked(s *subscriber) {
	if _, ok := r.subs[s]; ok {
		delete(r.subs, s)
		close(s.ch)
	}
}
