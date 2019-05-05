package cases

import "strconv"

type Culture struct {
	Name string

	// Symbols plus P
	F, I, N, V, T, W, P Symbol

	// Cardinal defines the plural rules for numbers indicating quantities.
	Cardinal Cases

	// Ordinal defines the plural rules for numbers indicating position
	// (first, second, etc.).
	Ordinal Cases

	// Vars only come from mod
	Vars []Var

	Tests UnitTests
}

func (c Culture) HasVars() bool {
	return len(c.Vars) != 0 ||
		c.F.Use() ||
		c.I.Use() ||
		c.N.Use() ||
		c.V.Use() ||
		c.T.Use() ||
		c.W.Use() ||
		c.P.Use()
}
func (c Culture) NeedFinvtw() bool      { return c.F.Use() || c.V.Use() || c.T.Use() || c.W.Use() }
func (c Culture) HasCardinal() bool     { return len(c.Cardinal) != 0 }
func (c Culture) HasOrdinal() bool      { return len(c.Ordinal) != 0 }
func (c Culture) HasTest() bool         { return c.HasCardinalTest() || c.HasOrdinalTest() }
func (c Culture) HasCardinalTest() bool { return len(c.Tests.Cardinal) != 0 }
func (c Culture) HasOrdinalTest() bool  { return len(c.Tests.Ordinal) != 0 }

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

type Case struct {
	Form string
	Cond string
}

type Cases []Case

func (s Cases) ToMap() (m map[string]*Case) {
	m = make(map[string]*Case, len(s))
	for i := range s {
		m[s[i].Form] = &s[i]
	}
	return
}

type Var struct {
	Symbol Symbol
	Mod    int
}

func (v Var) Name() string { return v.Symbol.Name() + strconv.Itoa(v.Mod) }

type UnitTest struct {
	Expected string
	Integers []string
	Decimals []string
}

type UnitTests struct {
	Cardinal []UnitTest
	Ordinal  []UnitTest
}
