package domain

type Improvement struct {
	Input  string
	Title  string
	Detail string
	Target float64
	Delta  int
	Score  int
	Grade  string
}

type ImprovementPlan struct {
	Available bool
	Score     int
	Grade     string
	Version   string
	Items     []Improvement
}

// Ориентир не является решением и не сохраняется в заявке.
type Preliminary struct {
	Version           string
	MonthlyRevenueKop int64
	Sources           []Source
	Items             []PreliminaryAmount
}

type PreliminaryAmount struct {
	Product   ProductCode
	AmountKop int64
}

type GuidanceRules struct {
	Version            string                  `yaml:"version"`
	Improvements       []ImprovementRule       `yaml:"improvements"`
	RevenueMultipliers map[ProductCode]float64 `yaml:"revenue_multipliers"`
}

type ImprovementRule struct {
	Input  string  `yaml:"input"`
	Target float64 `yaml:"target"`
	Title  string  `yaml:"title"`
	Detail string  `yaml:"detail"`
}
