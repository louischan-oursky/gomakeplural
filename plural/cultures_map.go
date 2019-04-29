package plural

import (
	"github.com/empirefox/makeplural/cases"
)

var culturesMap map[string]*cases.Culture

func CulturesMap() map[string]*cases.Culture {
	if culturesMap == nil {
		culturesMap = make(map[string]*cases.Culture, len(Cultures))
		for i := range Cultures {
			culturesMap[Cultures[i].Name] = &Cultures[i]
		}
	}
	return culturesMap
}

var othersMap map[string]bool

func IsOthers(cultrue string) bool {
	if othersMap == nil {
		othersMap = make(map[string]bool, len(Others))
		for _, c := range Others {
			othersMap[c] = true
		}
	}
	return othersMap[cultrue]
}
