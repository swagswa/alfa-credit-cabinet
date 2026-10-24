package domain

import "time"

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

// Payload ответ источника как есть, только у ok.
// At ставит база, при записи не нужен. Attempt не хранится, он только для живой карты.
type CheckResult struct {
	Source  Source
	Status  CheckStatus
	Payload []byte
	Error   string
	At      time.Time
	Attempt Attempt
}

// Attempt какая по счёту попытка запроса в источник и сколько их всего.
type Attempt struct {
	N, Of int
}
