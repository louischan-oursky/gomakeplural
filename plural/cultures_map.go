package plural

import (
	"golang.org/x/text/language"
)

var culturesMap map[language.Tag]*Culture

func CulturesMap() map[language.Tag]*Culture {
	if culturesMap == nil {
		culturesMap = make(map[language.Tag]*Culture, len(Cultures))
		for i := range Cultures {
			culturesMap[Cultures[i].Name] = &Cultures[i]
		}
	}
	return culturesMap
}

var othersMap map[language.Tag]bool

func IsOthers(cultrue language.Tag) bool {
	if othersMap == nil {
		othersMap = make(map[language.Tag]bool, len(Others))
		for _, c := range Others {
			othersMap[c] = true
		}
	}
	return othersMap[cultrue]
}
