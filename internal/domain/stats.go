package domain

import "time"

// Stats сводка дашборда банка по заявкам, поданным за период.
type Stats struct {
	Days      int
	Submitted int
	ByStatus  []StatusCount
	// Decided получили решение, из них AutoDecided без андеррайтера, Approved не отказ
	Decided     int
	AutoDecided int
	Approved    int
	// время от подачи до решения
	DecisionMedian time.Duration
	DecisionP90    time.Duration
	IssuedKop      int64
}

type StatusCount struct {
	Status  Status
	Count   int
	Overdue int
}
