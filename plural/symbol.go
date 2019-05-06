//go:generate stringer -type Symbol $GOFILE
package plural

type Symbol byte

func (s Symbol) Use() bool    { return s != 0 }
func (s Symbol) Name() string { return string(s) }

// where
// 	n  absolute value of the source number (integer and decimals)
// input
// 	i  integer digits of n.
// 	v  number of visible fraction digits in n, with trailing zeros.
// 	w  number of visible fraction digits in n, without trailing zeros.
// 	f  visible fractional digits in n, with trailing zeros (f = t * 10^(v-w))
// 	t  visible fractional digits in n, without trailing zeros.
//  p := w == 0
const U, F, I, N, V, T, W, P Symbol = 0, 'f', 'i', 'n', 'v', 't', 'w', 'p'
