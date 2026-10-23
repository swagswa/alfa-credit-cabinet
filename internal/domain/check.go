package domain

import "time"

// Source это внешний источник проверки заявки.
type Source string

const (
	SourceEGRUL    Source = "egrul"
	SourceFNS      Source = "fns"
	SourceBKI      Source = "bki"
	SourceInternal Source = "internal"
)

// Sources в порядке опроса, в нём же workflow собирает ответы.
var Sources = []Source{SourceEGRUL, SourceFNS, SourceBKI, SourceInternal}

type CheckStatus string

const (
	CheckOK          CheckStatus = "ok"
	CheckNotFound    CheckStatus = "not_found"
	CheckUnavailable CheckStatus = "unavailable"
)

// Коды стоп-факторов про сами источники, их ставит workflow до решения.
const (
	CodeNotFound          = "SF_NOT_FOUND"
	CodeSourceUnavailable = "SF_SOURCE_UNAVAILABLE"
)

// CheckResult это ответ источника. Payload ответ как есть, только у ok.
// At ставит база, при записи не нужен.
type CheckResult struct {
	Source  Source
	Status  CheckStatus
	Payload []byte
	Error   string
	At      time.Time
}
