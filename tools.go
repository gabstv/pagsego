package pagsego

import (
	"fmt"
)

func toPriceAmountStr(input float64) string {
	return fmt.Sprintf("%#.2f", input)
}
