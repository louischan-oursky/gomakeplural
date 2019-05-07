package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/elliotchance/pie/pie"
	"github.com/empirefox/makeplural/plural"
	"golang.org/x/text/language"
)

type (
	Source interface {
		Culture() string
		CultureId() string
		Code() string
	}

	Test interface {
		toString() string
	}

	FuncSource struct {
		culture, vars, impl string
	}

	UnitTestSource struct {
		culture string
		tests   []Test
	}

	UnitTest struct {
		ordinal         bool
		expected, value string
	}

	Op struct {
		previous_logic, left, operator, right, next_logic string
	}
)

func (x FuncSource) Culture() string {
	return x.culture
}

func (x FuncSource) CultureId() string {
	return sanitize(x.culture)
}

func (x FuncSource) Code() string {
	result := ""
	if "" != x.vars {
		result += x.vars + "\n"
	}
	result += x.impl
	return result
}

func (x UnitTestSource) Culture() string {
	return x.culture
}

func (x UnitTestSource) CultureId() string {
	return sanitize(x.culture)
}

func (x UnitTestSource) Code() string {
	var result []string
	for _, child := range x.tests {
		result = append(result, "\t\t"+child.toString())
	}
	return strings.Join(result, "\n")
}

func NewTestSource(name string, culture *plural.Culture) UnitTestSource {
	tests1 := NewTests(culture.Tests.Cardinal, false)
	tests2 := NewTests(culture.Tests.Ordinal, true)
	return UnitTestSource{name, append(tests1, tests2...)}
}

func NewTests(uts []plural.UnitTest, ordinal bool) []Test {
	length := 0
	for i := range uts {
		length += len(uts[i].Integers)
		length += len(uts[i].Decimals)
	}
	tests := make([]Test, 0, length)
	for _, ut := range uts {
		for _, v := range ut.Integers {
			tests = append(tests, UnitTest{ordinal, ut.Expected, v})
		}
		for _, v := range ut.Decimals {
			tests = append(tests, UnitTest{ordinal, ut.Expected, `"` + v + `"`})
		}
	}
	return tests
}

func (x UnitTest) toString() string {
	return fmt.Sprintf(
		"testNamedKey(t, fn, %s, `%s`, `%s`, %v)",
		x.value,
		x.expected,
		fmt.Sprintf("fn("+x.value+", %v)", x.ordinal),
		x.ordinal,
	)
}

func sanitize(input string) string {
	var result string
	for _, char := range input {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z':
			result += string(char)
		}
	}
	return result
}

func (x Op) conditions(culture *plural.Culture) []string {
	var result []string

	conditions := strings.Split(x.right, ",")
	for _, condition := range conditions {
		pos := strings.Index(condition, "..")

		if -1 != pos {
			lower_bound, upper_bound := condition[:pos], condition[pos+2:]
			lb, _ := strconv.Atoi(lower_bound)
			ub, _ := strconv.Atoi(upper_bound)

			r := rangeCondition(x.left, lb, ub, x.operator)
			if x.left[0] == 'n' {
				culture.P = plural.P
				r = "p && " + r
			}
			result = append(result, r)
		} else {
			result = append(result, fmt.Sprintf("%s %s %s", x.left, x.operator, condition))
		}
	}
	return result
}

func get(url, key string, headers *string) (map[string]map[string]string, error) {
	log.Print("GET ", url)

	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if 200 != response.StatusCode {
		return nil, fmt.Errorf(response.Status)
	}

	contents, err := ioutil.ReadAll(response.Body)

	var document map[string]map[string]json.RawMessage
	err = json.Unmarshal([]byte(contents), &document)
	if nil != err {
		return nil, err
	}

	if _, ok := document["supplemental"]; !ok {
		return nil, fmt.Errorf("Data does not appear to be CLDR data")
	}
	*headers += fmt.Sprintf("//\n// URL: %s\n", url)

	{
		var version map[string]string
		err = json.Unmarshal(document["supplemental"]["version"], &version)
		if nil != err {
			return nil, err
		}
		*headers += fmt.Sprintf("// %s\n", version["_number"])
	}

	var data map[string]map[string]string
	err = json.Unmarshal(document["supplemental"]["plurals-type-"+key], &data)
	if nil != err {
		return nil, err
	}
	return data, nil
}

func rangeCondition(varname string, lower, upper int, operator string) string {
	if operator == "!=" {
		return fmt.Sprintf("(%s < %d || %s > %d)", varname, lower, varname, upper)
	}
	return fmt.Sprintf("%s >= %d && %s <= %d", varname, lower, varname, upper)
}

func pattern2code(input string, culture *plural.Culture) []string {
	left, short, operator, logic := "", "", "", ""

	var ops []Op
	buf := ""
loop:
	for _, char := range input {
		switch char {
		default:
			buf += string(char)

		case '@':
			break loop

		case ' ':

		case '=':
			if "" != buf {
				left, operator, buf = buf, "==", ""
				short = toVar(left, culture)
			}

		case '!':
			left, operator, buf = buf, "!=", ""
			short = toVar(left, culture)
		}

		if "" != buf {
			pos := strings.Index(buf, "and")

			if -1 != pos {
				ops = append(ops, Op{logic, short, operator, buf[:pos], "AND"})
				buf, left, operator, logic = "", "", "", "AND"
			} else {
				pos = strings.Index(buf, "or")

				if -1 != pos {
					ops = append(ops, Op{logic, short, operator, buf[:pos], "OR"})
					buf, left, operator, logic = "", "", "", "OR"
				}
			}
		}
	}

	if "" != buf {
		ops = append(ops, Op{logic, short, operator, buf, ""})
	}

	if 1 == len(ops) {
		conditions := ops[0].conditions(culture)
		if "==" == ops[0].operator {
			return conditions
		} else {
			return []string{joinAnd(conditions)}
		}
	}

	var result []string
	var buffer []string

	buffer_length := 0
	for _, o := range ops {
		conditions := o.conditions(culture)
		logic = o.previous_logic
		nextLogic := o.next_logic
		operator := o.operator

		if "OR" == logic && buffer_length > 0 {
			result = append(result, strings.Join(buffer, " || "))
			buffer = []string{}
			buffer_length = 0
		}

		if ("" == logic && "OR" == nextLogic) || ("OR" == logic && "OR" == nextLogic) || ("OR" == logic && "" == nextLogic) {
			if "==" == operator {
				buffer = append(buffer, conditions...)
			} else {
				buffer = append(buffer, joinAnd(conditions))
			}
			buffer_length = len(buffer)
		} else if "AND" == logic && ("AND" == nextLogic || "" == nextLogic) {
			if "==" == operator {
				joinTo(buffer, buffer_length-1, joinOr(conditions))
			} else {
				joinTo(buffer, buffer_length-1, joinAnd(conditions))
			}
		} else if "" == logic && "AND" == nextLogic {
			if "==" == operator {
				buffer = append(buffer, joinOr(conditions))
			} else {
				buffer = append(buffer, joinAnd(conditions))
			}
			buffer_length = len(buffer)
		} else if "OR" == logic && "AND" == nextLogic {
			if "==" == operator {
				if len(conditions) > 1 {
					buffer = append(buffer, joinOr(conditions))
				} else {
					buffer = append(buffer, conditions...)
				}
			} else {
				buffer = append(buffer, joinAnd(conditions))
			}
			buffer_length = len(buffer)
		} else if "AND" == logic && "OR" == nextLogic {
			if "==" == operator {
				joinTo(buffer, buffer_length-1, joinOr(conditions))
			} else {
				joinTo(buffer, buffer_length-1, joinAnd(conditions))
			}
		}
	}

	if len(buffer) > 0 {
		if "OR" == logic {
			result = append(result, strings.Join(buffer, " || "))
		} else {
			result = append(result, joinAnd(buffer))
		}
	}
	return result
}

func joinTo(data []string, idx int, toAppend string) {
	p := strings.HasPrefix(toAppend, "p && ")
	toAppend = strings.TrimPrefix(toAppend, "p && ")
	if !strings.HasPrefix(data[idx], "p && ") && p {
		data[idx] = "p && " + data[idx]
	}
	data[idx] += " && " + toAppend
}

func joinAnd(data []string) string {
	return strings.Join(cleanAnd(data), " && ")
}

func cleanAnd(data []string) []string {
	p := false
	for i, s := range data {
		p = p || strings.HasPrefix(s, "p && ")
		data[i] = strings.TrimPrefix(s, "p && ")
	}
	if p {
		data[0] = "p && " + data[0]
	}
	return data
}

func joinOr(data []string) string {
	if len(data) > 1 {
		return "(" + strings.Join(data, " || ") + ")"
	}
	return data[0]
}

func rule2code(key string, data map[string]string, padding string, culture *plural.Culture, ordinal bool) string {
	if input, ok := data["pluralRule-count-"+key]; ok {
		result := ""

		if "other" == key {
			if 1 == len(data) {
				return "return \"other\"\n"
			}
			result += "default:\n"
		} else {
			cond := strings.Join(pattern2code(input, culture), " || ")
			if ordinal {
				culture.Ordinal = append(culture.Ordinal, plural.Case{Form: key, Cond: cond})
			} else {
				culture.Cardinal = append(culture.Cardinal, plural.Case{Form: key, Cond: cond})
			}
			result += "\n" + "case " + cond + ":\n"
		}
		result += "\treturn \"" + key + "\"\n"
		return result
	}
	return ""
}

func map2code(data map[string]string, padding string, culture *plural.Culture, ordinal bool) string {
	if 1 == len(data) {
		return rule2code("other", data, padding, culture, ordinal)
	}
	result := "switch {\n"
	result += rule2code("other", data, padding, culture, ordinal)
	result += rule2code("zero", data, padding, culture, ordinal)
	result += rule2code("one", data, padding, culture, ordinal)
	result += rule2code("two", data, padding, culture, ordinal)
	result += rule2code("few", data, padding, culture, ordinal)
	result += rule2code("many", data, padding, culture, ordinal)
	result += "}\n"
	return result
}

func splitValues(input string) []string {
	var result []string

	pos := -1
	for idx, char := range input {
		switch {
		case (char >= '0' && char <= '9') || '.' == char:
			if -1 == pos {
				pos = idx
			}

		case ' ' == char || ',' == char || '~' == char:
			if -1 != pos {
				result = append(result, input[pos:idx])
				pos = -1
			}
		}
	}

	if -1 != pos {
		result = append(result, input[pos:])
	}
	return result
}

func pattern2test(expected, input string, culture *plural.Culture, ordinal bool) {
	ut := plural.UnitTest{Expected: expected}
	patterns := strings.Split(input, "@")
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "integer") {
			ut.Integers = splitValues(pattern[8:])
		} else if strings.HasPrefix(pattern, "decimal") {
			ut.Decimals = splitValues(pattern[8:])
		}
	}
	if ordinal {
		culture.Tests.Ordinal = append(culture.Tests.Ordinal, ut)
	} else {
		culture.Tests.Cardinal = append(culture.Tests.Cardinal, ut)
	}
}

func map2test(ordinals, plurals map[string]string, culture *plural.Culture) {
	for _, rule := range []string{"one", "two", "few", "many", "zero", "other"} {
		if input, ok := ordinals["pluralRule-count-"+rule]; ok {
			pattern2test(rule, input, culture, true)
		}

		if input, ok := plurals["pluralRule-count-"+rule]; ok {
			pattern2test(rule, input, culture, false)
		}
	}
}

func culture2code(ordinals, plurals map[string]string, padding string, culture *plural.Culture) (string, string) {
	var code string

	if nil == ordinals {
		code = map2code(plurals, padding, culture, false)
	} else {
		code = "if ordinal {\n"
		code += map2code(ordinals, padding+"\t", culture, true)
		code += "}\n\n"
		code += map2code(plurals, padding, culture, false)
	}
	map2test(ordinals, plurals, culture)

	str_vars := ""

	if culture.HasVars() {
		// http://unicode.org/reports/tr35/tr35-numbers.html#Operands
		//
		// Symbol	Value
		// n	    absolute value of the source number (integer and decimals).
		// i	    integer digits of n.
		// v	    number of visible fraction digits in n, with trailing zeros.
		// w	    number of visible fraction digits in n, without trailing zeros.
		// f	    visible fractional digits in n, with trailing zeros.
		// t	    visible fractional digits in n, without trailing zeros.
		if culture.P.Use() && !culture.W.Use() {
			culture.N = plural.N
		}

		if culture.F.Use() || culture.V.Use() || culture.T.Use() || culture.W.Use() {
			if culture.P.Use() {
				culture.W = plural.W
			}
			str_vars += fmt.Sprintf("%s, %s, %s, %s, %s, %s := finvtw(value)\n",
				getSymbolName(culture.F),
				getSymbolName(culture.I),
				getSymbolName(culture.N),
				getSymbolName(culture.V),
				getSymbolName(culture.T),
				getSymbolName(culture.W))
			if culture.P.Use() {
				str_vars += "p := w == 0\n"
			}
		} else {
			if culture.N.Use() {
				if culture.I.Use() {
					str_vars += "flt := float(value)\n"
					str_vars += "n := math.Abs(flt)\n"
					str_vars += "i := int64(flt)\n"
				} else {
					str_vars += "n := math.Abs(float(value))\n"
				}
			} else if culture.I.Use() {
				str_vars += "i := int64(float(value))\n"
			}
			if culture.P.Use() {
				if culture.I.Use() {
					str_vars += "p := float64(i) == n\n"
				} else {
					str_vars += "p := float64(int64(n)) == n\n"
				}
			}
		}

		for _, v := range culture.Vars {
			str_vars += v.Name() + " := " + toVarExpr(v) + "\n"
		}
		if len(culture.Vars) == 0 {
			culture.Vars = nil
		}
	}
	return str_vars, code
}

func toVar(expr string, culture *plural.Culture) string {
	var v plural.Var
	if pos := strings.Index(expr, "%"); -1 != pos {
		k, m := expr[:pos], expr[pos+1:]
		if len(k) != 1 {
			log.Fatalln("symbol mod err:", expr)
		}
		mod, err := strconv.Atoi(m)
		if err != nil {
			log.Fatalf("mod err:\n", expr)
		}
		v = plural.Var{Symbol: setSymbol(k[0], culture), Mod: mod}
	} else {
		if len(expr) != 1 {
			log.Fatalln("symbol length err:", expr)
		}
		return setSymbol(expr[0], culture).Name()
	}

	for _, e := range culture.Vars {
		if e == v {
			return v.Name()
		}
	}

	culture.Vars = append(culture.Vars, v)
	return v.Name()
}

var symbols = map[byte]bool{
	'f': true,
	'i': true,
	'n': true,
	'v': true,
	't': true,
	'w': true,
	'p': true,
}

func toSymbol(s byte) plural.Symbol {
	if !symbols[s] {
		log.Fatalf("toSymbol err: %s, %d\n", s, s)
	}
	return plural.Symbol(s)
}

func setSymbol(s byte, culture *plural.Culture) plural.Symbol {
	b := toSymbol(s)
	switch b {
	case plural.F:
		culture.F = b
	case plural.I:
		culture.I = b
	case plural.N:
		culture.N = b
	case plural.V:
		culture.V = b
	case plural.T:
		culture.T = b
	case plural.W:
		culture.W = b
	case plural.P:
		culture.P = b
	}
	return b
}

func getSymbolName(s plural.Symbol) string {
	if s.Use() {
		return toSymbol(byte(s)).Name()
	}
	return "_"
}

func toVarExpr(v plural.Var) string {
	if v.Symbol == 'n' {
		return fmt.Sprintf("mod(n, %d)", v.Mod)
	}
	return string(v.Symbol) + " % " + strconv.Itoa(v.Mod)
}

func isRuleParsed(culture string, in []string, allPlurals, allOrdinals map[string]map[string]string) (string, bool) {
	plurals := allPlurals[culture]
	ordinals := allOrdinals[culture]
	for _, lang := range in {
		if reflect.DeepEqual(plurals, allPlurals[lang]) && reflect.DeepEqual(ordinals, allOrdinals[lang]) {
			return lang, true
		}
	}
	return "", false
}

func createGoFiles(headers string, allPlurals, allOrdinals map[string]map[string]string) error {
	var cultures []string
	if "*" == *user_culture {
		for culture, _ := range allPlurals {
			cultures = append(cultures, culture)
		}
	} else {
		for _, culture := range strings.Split(*user_culture, ",") {
			culture = strings.TrimSpace(culture)

			if _, ok := allPlurals[culture]; !ok {
				return fmt.Errorf("Aborted, `%s` not found...", culture)
			}
			cultures = append(cultures, culture)
		}
	}
	sort.Strings(cultures)

	if len(cultures) == 0 {
		return fmt.Errorf("Not enough data to create source...")
	}

	var items []Source
	var tests []Source

	datas := make([]*plural.Culture, 0, len(cultures))
	others := make(pie.Strings, 0, len(cultures))
	culturesMap := make(map[language.Tag]*plural.Culture, len(cultures))
	othersMap := make(map[language.Tag]bool, len(cultures))
	for i, culture := range cultures {
		t := language.MustParse(culture)
		log.Print(culture, "=>", t.String())

		if data, ok := culturesMap[t]; ok {
			data.Langs = append(data.Langs, culture)
			if t.String() != culture {
				data.Langs = append(data.Langs, t.String())
			}
			continue
		}
		if othersMap[t] {
			others = others.Append(culture)
			if t.String() != culture {
				others = others.Append(t.String())
			}
			continue
		}

		plurals, ok := allPlurals[culture]
		if !ok {
			log.Println(" \u2717 - Plural not defined")
			continue
		}

		if _, ok = plurals["pluralRule-count-other"]; !ok {
			log.Println(" \u2717 - Plural missing mandatory `other` choice...")
			continue
		}

		ordinals, ok := allOrdinals[culture]
		if ok {
			if _, ok = ordinals["pluralRule-count-other"]; !ok {
				log.Println(" \u2717 - Ordinal missing the mandatory `other` choice...")
				continue
			}
		}

		var dataAdded bool
		if exist, ok := isRuleParsed(culture, cultures[:i], allPlurals, allOrdinals); ok {
			if data, ok := culturesMap[language.MustParse(exist)]; ok {
				culturesMap[t] = data
				data.Langs = append(data.Langs, culture)
				if t.String() != culture {
					data.Langs = append(data.Langs, t.String())
				}
			} else {
				others = others.Append(culture)
				if t.String() != culture {
					others = others.Append(t.String())
				}
				othersMap[t] = true
			}
			dataAdded = true
		}

		data := plural.Culture{
			Langs:    []string{culture},
			Cardinal: make(plural.Cases, 0, 5),
			Ordinal:  make(plural.Cases, 0, 5),
			Vars:     make([]plural.Var, 0, 8),
		}
		vars, code := culture2code(ordinals, plurals, "\t\t", &data)
		if !dataAdded {
			if data.HasCardinal() || data.HasOrdinal() {
				datas = append(datas, &data)
				if t.String() != culture {
					data.Langs = append(data.Langs, t.String())
				}
				culturesMap[t] = &data
			} else {
				others = others.Append(culture)
				if t.String() != culture {
					others = others.Append(t.String())
				}
				othersMap[t] = true
			}
		}
		items = append(items, FuncSource{t.String(), vars, code})

		if data.HasTest() {
			tests = append(tests, NewTestSource(t.String(), &data))
		}
	}

	for _, data := range datas {
		data.Langs = []string(pie.Strings(data.Langs).Unique().Sort())
	}
	others = others.Unique().Sort().Unselect(func(lang string) bool {
		return lang == "und"
	})

	err := createPluralsData("plural/cultures.go", &culturesTplData{
		Headers:  headers,
		Cultures: datas,
		Others:   []string(others),
	})
	if err != nil {
		return err
	}

	if len(tests) > 0 {
		err := createSource("plural_test.tmpl", "plural/func_test.go", headers, tests)
		if nil != err {
			return err
		}
	}
	return createSource("plural.tmpl", "plural/func.go", headers, items)
}

const culturesTplStr = `// Generated by https://github.com/empirefox/makeplural
// at {{ now.UTC | printf "%v" }}
{{ .Headers }}
package plural

var Info = PluralInfo{
	Cultures: []Culture{
		{{- range .Cultures }}
		{{ template "culture" . }},
		{{- end }}
	},
	Others: {{ .Others | printf "%#v" }},
}
`

const cultureTplStr = `{
	Langs: {{ .Langs | printf "%#v" }},
	{{ range . | symbols }} {{ if .Use }}{{.String}}:{{.String}},{{ end }} {{ end }}
	{{ if .Cardinal }} Cardinal: {{ template "cases" .Cardinal }}, {{ end }}
	{{ if .Ordinal }} Ordinal: {{ template "cases" .Ordinal }}, {{ end }}
	{{ if .Vars }} Vars: {{ template "vars" .Vars }}, {{ end }}
	{{ if .Tests }} Tests: {{ template "tests" .Tests }}, {{ end }}
}`

const casesTplStr = `Cases{
	{{- range . }}
	{ Form: "{{ .Form }}", Cond: "{{ .Cond }}" },
	{{- end }}
}`

const varsTplStr = `[]Var{
	{{- range . }}
	{ Symbol: {{ .Symbol.String }}, Mod: {{ .Mod }} },
	{{- end }}
}`

const testsTplStr = `UnitTests{
	{{ if .Cardinal }} Cardinal: {{ template "testCases" .Cardinal }}, {{ end }}
	{{ if .Ordinal }} Ordinal: {{ template "testCases" .Ordinal }}, {{ end }}
}`

const testCasesTplStr = `[]UnitTest{
	{{- range . }}
	{
		Expected: "{{ .Expected }}",
		{{ if .Integers }} Integers: {{ .Integers | printf "%#v" }}, {{ end }}
		{{ if .Decimals }} Decimals: {{ .Decimals | printf "%#v" }}, {{ end }}
	},
	{{- end }}
}`

func symbolsTplFunc(c *plural.Culture) []plural.Symbol {
	return []plural.Symbol{c.F, c.I, c.N, c.V, c.T, c.W, c.P}
}

var culturesTpl = template.Must(template.New("cultures").
	Funcs(sprig.TxtFuncMap()).
	Funcs(template.FuncMap{"symbols": symbolsTplFunc}).
	Parse(culturesTplStr))

func init() {
	template.Must(culturesTpl.New("culture").Parse(cultureTplStr))
	template.Must(culturesTpl.New("cases").Parse(casesTplStr))
	template.Must(culturesTpl.New("vars").Parse(varsTplStr))
	template.Must(culturesTpl.New("tests").Parse(testsTplStr))
	template.Must(culturesTpl.New("testCases").Parse(testCasesTplStr))
}

type culturesTplData struct {
	Headers  string
	Cultures []*plural.Culture
	Others   []string
}

func createPluralsData(dest_filepath string, data *culturesTplData) error {
	file, err := createSourceFile(dest_filepath)
	if nil != err {
		return err
	}
	defer file.Close()

	err = culturesTpl.Execute(file, data)
	if err != nil {
		return err
	}

	return file.Save()
}

func createSource(tmpl_filepath, dest_filepath, headers string, items []Source) error {
	source, err := template.ParseFiles(tmpl_filepath)
	if nil != err {
		return err
	}

	file, err := createSourceFile(dest_filepath)
	if nil != err {
		return err
	}
	defer file.Close()

	err = source.Execute(file, struct {
		Headers   string
		Timestamp string
		Items     []Source
	}{
		headers,
		time.Now().Format(time.RFC3339),
		items,
	})
	if err != nil {
		return err
	}

	return file.Save()
}

type sourceFile struct {
	bytes.Buffer
	pres []func(b []byte) []byte
	f    *os.File
}

func createSourceFile(name string, pres ...func(b []byte) []byte) (*sourceFile, error) {
	f, err := os.Create(name)
	if nil != err {
		return nil, err
	}
	return &sourceFile{f: f, pres: pres}, nil
}
func (sf *sourceFile) Save() error {
	b := sf.Bytes()
	for _, pre := range sf.pres {
		if pre != nil {
			b = pre(b)
		}
	}
	b, err := format.Source(b)
	if err != nil {
		return err
	}
	_, err = sf.f.Write(b)
	return err
}
func (sf *sourceFile) Close() error { return sf.f.Close() }

var user_culture = flag.String("culture", "*", "Culture subset")

// TODO dont know howto really fix it
var fixOrdinalsKwTwo = "n % 100 = 2,22,42,62,82 @integer 2, 22, 42, 62, 82, 102, 122, 142, 1002, … @decimal 2.0, 22.0, 42.0, 62.0, 82.0, 102.0, 122.0, 142.0, 1002.0, …"

func main() {
	flag.Parse()

	var headers string

	ordinals, err := get("https://github.com/unicode-cldr/cldr-core/raw/master/supplemental/ordinals.json", "ordinal", &headers)
	if nil != err {
		log.Println(" \u2717")
		log.Fatalln(err)
	}

	log.Println(" \u2713")
	plurals, err := get("https://github.com/unicode-cldr/cldr-core/raw/master/supplemental/plurals.json", "cardinal", &headers)
	if nil != err {
		log.Println(" \u2717")
		log.Fatalln(err)
	}

	plurals["kw"]["pluralRule-count-two"] = fixOrdinalsKwTwo
	log.Println(" \u2713")

	err = createGoFiles(headers, plurals, ordinals)
	if nil != err {
		log.Fatalln(err, "(╯°□°）╯︵ ┻━┻")
	}

	log.Println("Succeed (ッ)")
}
