package domain

import "time"

type CompanyKind string

const (
	KindUL CompanyKind = "UL"
	KindIP CompanyKind = "IP"
)

// Company это выписка ЕГРЮЛ/ЕГРИП для автоподстановки в анкету.
// Director и KPP у ИП пустые, учредителей у ИП нет.
type Company struct {
	INN          string
	OGRN         string
	KPP          string
	Kind         CompanyKind
	FullName     string
	ShortName    string
	Status       string
	RegisteredAt time.Time
	Address      string
	Okved        Okved
	Director     *Director
	Founders     []Founder
	Simulated    bool // из генератора симулятора, а не из фикстур
}

type Okved struct {
	Code string
	Name string
}

type Director struct {
	FIO      string
	Position string
}

type Founder struct {
	Name     string
	SharePct float64
}
