package plural

import (
	"strconv"

	"golang.org/x/text/language"
)

type PluralInfo struct {
	Cultures []Culture
	Others   []language.Tag

	EqualCultures []string
	EqualOthers   []string

	culturesMap map[language.Tag]*Culture
	othersMap   map[language.Tag]bool
}

func (pi *PluralInfo) CulturesMap() map[language.Tag]*Culture {
	if pi.culturesMap == nil {
		pi.culturesMap = make(map[language.Tag]*Culture, len(pi.Cultures))
		for i := range pi.Cultures {
			pi.culturesMap[pi.Cultures[i].Name] = &pi.Cultures[i]
		}
	}
	return pi.culturesMap
}

func (pi *PluralInfo) IsOthers(cultrue language.Tag) bool {
	if pi.othersMap == nil {
		pi.othersMap = make(map[language.Tag]bool, len(pi.Others))
		for _, c := range pi.Others {
			pi.othersMap[c] = true
		}
	}
	return pi.othersMap[cultrue]
}

type Culture struct {
	Name language.Tag

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
