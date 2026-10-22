package domain

// Весовые коэффициенты ФНС для контрольных цифр ИНН. У ИП их две:
// 11-я считается по первым десяти, 12-я по первым одиннадцати.
var (
	innWeights10 = []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
	innWeights11 = []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	innWeights12 = []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
)

const (
	innLenUL = 10
	innLenIP = 12
)

// ValidINN проверяет длину (10 у юрлица, 12 у ИП) и контрольные цифры.
func ValidINN(inn string) bool {
	d := make([]int, len(inn))
	for i, r := range inn {
		if r < '0' || r > '9' {
			return false
		}
		d[i] = int(r - '0')
	}
	switch len(d) {
	case innLenUL:
		return innCheck(d, innWeights10) == d[9]
	case innLenIP:
		return innCheck(d, innWeights11) == d[10] && innCheck(d, innWeights12) == d[11]
	}
	return false
}

func innCheck(d, weights []int) int {
	sum := 0
	for i, w := range weights {
		sum += d[i] * w
	}
	return sum % 11 % 10 //nolint:mnd // так у ФНС: остаток от 11, десятка превращается в 0
}
