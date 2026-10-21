// Package activity: шаги процессов Temporal. Логика в usecase, activity его
// только вызывает; ошибки сети и 5xx возвращаем как есть, повторит RetryPolicy.
package activity

// Activities держит зависимости шагов: usecase и клиенты integration.
type Activities struct{}
