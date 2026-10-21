// Package rbac: права ролей на эндпоинты. Субъект это роль, объект путь,
// действие HTTP-метод. Владение заявкой и права на переходы сюда не входят:
// первое проверяется в SQL, второе таблицей переходов.
package rbac

import (
	_ "embed"
	"fmt"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	stringadapter "github.com/casbin/casbin/v3/persist/string-adapter"
)

// Anonymous это роль запроса без сессии: так публичные операции живут в той же политике.
const Anonymous = "anonymous"

var (
	//go:embed model.conf
	modelConf string
	//go:embed policy.csv
	policyCSV string
)

type Enforcer struct {
	e *casbin.Enforcer
}

func New() (*Enforcer, error) {
	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return nil, fmt.Errorf("rbac model: %w", err)
	}
	e, err := casbin.NewEnforcer(m, stringadapter.NewAdapter(policyCSV))
	if err != nil {
		return nil, fmt.Errorf("rbac policy: %w", err)
	}
	return &Enforcer{e: e}, nil
}

func (e *Enforcer) Allowed(role, path, method string) (bool, error) {
	ok, err := e.e.Enforce(role, path, method)
	if err != nil {
		return false, fmt.Errorf("enforce: %w", err)
	}
	return ok, nil
}
