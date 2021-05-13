package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
	"tnd/pkg/encoding/strictxml"
)

type FromToKey struct {
	From string
	To   string
}

var forceArrayType = map[string]bool{}
var specialName map[string]string
var fromToKeyMap []FromToKey

// const inputFile = `C:\TND_DATA\workData\2019-01-29\pnd52\PND52_XML_Public_2562_05062020.csv`

// const inputFile = `C:\TND_DATA\workData\2019-01-29\pnd55\XML Public Pnd55.csv`

const xmlNameSpace = "urn:schemas-rd-go-th:xml-services:common"

var specFile = flag.String("spec", ``, `xml spec file in csv format`)
var specContextLength = flag.Int("specContextLength", 8, "length of xml hierarchy")
var tempJSONFile = flag.String("printJSONSpec", ``, "file to print spec in json format")
var javaParserFile = flag.String("javaParser", ``, "print part of java parser into file")
var jsonTestDataFile = flag.String("jsonTestData", ``, "json file to copy test data from")
var xmlTestDataFile = flag.String("xmlTestData", ``, "xml file to contain test data")
var xmlNameSubstitutionFileName = flag.String("nameSubstitution", ``, `json file represent name substitution`)
var xmlNameMapping = flag.String("xmlNameMapping", ``, `json file represent prefix name mapping`)
var xsdFile = flag.String("xsdFile", ``, `filepath to output generated xsd file`)

type fromToKeySorter []FromToKey

func (ftk fromToKeySorter) Len() int {
	return len(ftk)
}
func (ftk fromToKeySorter) Swap(a, b int) {
	ftk[a], ftk[b] = ftk[b], ftk[a]
}
func (ftk fromToKeySorter) Less(a, b int) bool {
	return ftk[a].From < ftk[b].From
}

func readNameMappingFile() {
	specialName = make(map[string]string)
	file, err := os.Open(*xmlNameMapping)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	var nameMapping map[string]interface{}
	err = json.NewDecoder(file).Decode(&nameMapping)
	if err != nil {
		panic(err)
	}
	add := func(key, value string) {
		fromToKeyMap = append(fromToKeyMap, FromToKey{From: key, To: value})
	}
	for key, v := range nameMapping {
		add(key, v.(string))
	}
	file, err = os.Open(*xmlNameSubstitutionFileName)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	err = json.NewDecoder(file).Decode(&nameMapping)
	if err != nil {
		panic(err)
	}
	add = func(key, value string) {
		specialName[key] = value
	}
	for key, v := range nameMapping {
		add(key, v.(string))
	}
	sort.Sort(fromToKeySorter(fromToKeyMap))
}

type JsonOutput struct {
	Index       string
	FromKey     string
	ToKey       string `json:",omitempty"`
	Description string
	Type        string
	MaxLength   string
	Multiple    string
	Input       string
}

type AnyXML struct {
	XMLName xml.Name
	Nodes   []AnyXML   `xml:",any"`
	Attrs   []xml.Attr `xml:",attr,any"`
	Data    string     `xml:",chardata"`
}

func checkFlag() bool {
	if *specFile == "" {
		log.Println("Need Xml Spec file")
		return false
	}
	if *xmlNameMapping == "" {
		log.Println("Need xml name mapping file")
		return false
	}
	if *xmlNameSubstitutionFileName == "" {
		log.Println("Need xml name subtitution file")
		return false
	}
	return true
}

func main() {
	flag.Parse()
	if !checkFlag() {
		return
	}
	excelCsvToJson()
	readNameMappingFile()
	modifyRule()
	if *javaParserFile != "" {
		createParser(*javaParserFile)
	}
	if *xsdFile != "" {
		createXsd(*xsdFile)
		prettyPrintXML(*xsdFile)
	}
	if *xmlTestDataFile != "" {
		if *jsonTestDataFile != "" {
			jsonToXML()
		} else {
			createTestData()
		}
	}
}

func createTestData() {
	jsonInput := readJson()
	outFile, err := os.Create(*xmlTestDataFile)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	root := AnyXML{XMLName: xml.Name{Local: "rd:RdForm"}}
	root.Attrs = append(root.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:rd"}, Value: xmlNameSpace})
	var arrayNames []string
	setXMLValue := func(name string, value string) {
		tokens := strings.Split(name, ".")
		current := &root
	tokensLoop:
		for _, n := range tokens {
			for i := range current.Nodes {
				if current.Nodes[i].XMLName.Local[3:] == n {
					current = &current.Nodes[i]
					continue tokensLoop
				}
			}
			current.Nodes = append(current.Nodes, AnyXML{
				XMLName: xml.Name{Local: "rd:" + n},
			})
			current = &current.Nodes[len(current.Nodes)-1]
		}
		current.Data = value
	}
	duplicateXMLTag := func(name string) {
		tokens := strings.Split(name, ".")
		var parent *AnyXML
		current := &root
		currentIdx := 0
	tokensLoop:
		for _, n := range tokens {
			for i := range current.Nodes {
				if current.Nodes[i].XMLName.Local[3:] == n {
					parent = current
					currentIdx = i
					current = &current.Nodes[i]
					continue tokensLoop
				}
			}
			panic("Can't duplicate array tag")
		}
		parent.Nodes = append(parent.Nodes[:currentIdx], append([]AnyXML{*current}, parent.Nodes[currentIdx:]...)...)
	}

	for _, field := range jsonInput {
		if strings.HasPrefix(field.Type, "Decimal") {
			field.Type = "Number"
		}
		switch field.Type {
		case "Array":
			arrayNames = append(arrayNames, field.FromKey)
		case "Date":
			setXMLValue(field.FromKey, "2020-01-02")
		case "Number":
			setXMLValue(field.FromKey, "8.20")
		case "String":
			setXMLValue(field.FromKey, "Test")
		}
	}
	for _, n := range arrayNames {
		duplicateXMLTag(n)
	}
	encoder := xml.NewEncoder(outFile)
	encoder.Indent("", "    ")
	encoder.Encode(root)
	encoder.Flush()
}

func createParser(parserFilePath string) {
	jsonInput := readJson()
	outFile, err := os.Create(parserFilePath)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()
	var parentArray []string
	for _, field := range jsonInput {
		// PND52 have some different in json and xml,so ignore all grossReceipts and implements it manually
		if strings.HasPrefix(field.FromKey, "TaxFormDetail.Calculate.GrossReceiptsAndTaxComputation.GrossReceiptsBeforeDec.Detail") {
			continue
		}
		if field.ToKey == "" {
			continue
		}
		// set filingNo to string
		switch field.FromKey {
		case "TaxForm.Filing.FilingNo", "TaxForm.Filing.FilingType":
			field.Type = "String"
		}
		if field.Type == "Object" && !forceArrayType[field.FromKey] {
			continue
		}
		output := ""
		afterArrayName := field.ToKey
		var arrayName string
		var useArray bool
		if (len(parentArray)) > 0 {
			arrayName = parentArray[len(parentArray)-1]
			if strings.HasPrefix(field.ToKey, arrayName) {
				afterArrayName = field.ToKey[len(arrayName)+1:]
				useArray = true
			}
		}

		if useArray {
			output += "\tuseArray(\"" + arrayName + "\");\n"
		} else {
			output += "\tuseRoot();\n"
		}
		if strings.HasPrefix(field.Type, "Decimal") {
			field.Type = "Number"
		}
		if field.Type == "Array" || forceArrayType[field.FromKey] {
			parentArray = append(parentArray, field.ToKey)
			output += "\tfinalizeArray(\"" + afterArrayName + "\", \"" + field.ToKey + "\");\n"
		} else {
			switch field.Type {
			case "Date":
				output += "\tsetDate(\"" + afterArrayName + "\", value);\n"
			case "Number":
				output += "\tsetNumber(\"" + afterArrayName + "\", value);\n"
			case "String":
				output += "\tsetString(\"" + afterArrayName + "\", value);\n"
			case "Object":
			case "Boolean":
				output += "\tsetBoolean(\"" + afterArrayName + "\", value);\n"
			default:
				panic("unknown field type " + field.Type)
			}
		}
		orPanic(outFile.WriteString("if (\"RdForm." + field.FromKey + "\".equals(name)) {\n"))
		orPanic(outFile.WriteString(output))
		orPanic(outFile.WriteString("} else "))
	}
	orPanic(outFile.WriteString("{}"))
}

func modifyRule() {
	jsonInput := readJson()
	var jsonOutput []JsonOutput
	for _, field := range jsonInput {
		if field.ToKey != "" {
			continue
		}
		for _, formToKey := range fromToKeyMap {
			k := formToKey.From
			v := formToKey.To
			if strings.HasPrefix(field.FromKey, k) {
				field.ToKey = v + strings.Join(stringArrayMap(strings.Split(field.FromKey[len(k):], "."), func(str string) string {
					if newValue, ok := specialName[str]; ok {
						return newValue
					}
					str = strings.Join(stringArrayMap(strings.Split(str, "_"), func(str string) string { // make first letter of each word separated by under score to lower case.
						if len(str) > 0 {
							return strings.ToLower(str[:1]) + str[1:]
						}
						return str
					}), "_")
					return str
					// rs := []rune(str)
					// if len(rs) > 0 {
					// 	rs[0] = unicode.ToLower(rs[0])
					// 	if rs[len(rs)-1] >= '0' && rs[len(rs)-1] <= '9' { // put leading number at the end. not used anymore
					// 		str = string(rs)
					// 		tokens := strings.Split(str, "_")
					// 		for i := 0; i < len(tokens)/2; i++ {
					// 			tokens[i], tokens[len(tokens)-i-1] = tokens[len(tokens)-i-1], tokens[i]
					// 		}
					// 		str = strings.Join(tokens, "_")
					// 		rs = []rune(str)
					// 	}
					// }
					// return string(rs)
				}), ".")
				if strings.HasSuffix(field.FromKey, ".Detail") { // make all Trailing ".Detail" type with max more than 1 to Array
					multipleTokens := strings.Split(field.Multiple, "…")
					if len(multipleTokens) > 1 {
						max := multipleTokens[1][:strings.Index(multipleTokens[1], "]")]
						switch max {
						case "0", "1":
						default:
							if field.Type == "Object" {
								field.Type = "Array"
							}
						}
					} else {
						log.Println("multiple wrong format")
					}
				}
			}
		}
		if field.ToKey == "" {
			log.Println("ignore xml", field.FromKey)
		}
		jsonOutput = append(jsonOutput, field)
	}
	writeJson(jsonOutput)
}

func excelCsvToJson() {
	contextLength := *specContextLength
	file, err := os.Open(*specFile)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	_, err = reader.Read()
	if err != nil {
		panic(err)
	}
	context := make([]string, contextLength)
	var result []JsonOutput
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		index := record[0]
		currentContext := 0
		for i, r := range record[1 : 1+contextLength] {
			if r != "" {
				currentContext = i
				for j := range context[i:] {
					context[i+j] = ""
				}
				break
			}
		}
		context[currentContext] = stripTagRd(record[1+contextLength])
		fromKey := joinStripEmpty(context)
		description := record[2+contextLength]
		Type := strings.TrimSpace(record[3+contextLength])
		max := strings.TrimSpace(record[4+contextLength])
		multiple := strings.TrimSpace(record[5+contextLength])
		input := record[6+contextLength]
		// if Type == "Object" {
		// 	continue
		// }
		{
			isUsed := false
			switch {
			case input == "NU": // Not used input
			case Type == "": // Empty Row and table name
			case Type == "Type" && index == "Index": // Header
			default:
				isUsed = true
			}
			if !isUsed {
				continue
			}
		}
		result = append(result, JsonOutput{
			Description: description,
			FromKey:     fromKey,
			Index:       index,
			MaxLength:   max,
			Multiple:    multiple,
			Type:        Type,
			Input:       input,
		})
	}
	writeJson(result)
}

func joinStripEmpty(strs []string) string {
	result := ""
	prefix := ""
	for _, str := range strs {
		if str != "" {
			result += prefix + strings.TrimSpace(str)
			prefix = "."
		}
	}
	return result
}

func stripTagRd(name string) string {
	name = strings.TrimSpace(name)
	if len(name) >= 4 && name[:4] == "<rd:" {
		return name[4 : len(name)-1]
	}
	if len(name) >= 1 && name[:1] == "<" {
		return name[1 : len(name)-1]
	}
	return name
}

func stringArrayMap(strs []string, f func(str string) string) []string {
	var result []string
	for _, str := range strs {
		str = f(str)
		result = append(result, str)
	}
	return result
}

func orPanic(a ...interface{}) {
	if len(a) > 0 && a[len(a)-1] != nil {
		panic(a[len(a)-1])
	}
}

func prettyPrintXML(location string) {
	var data []byte
	func() {
		inFile, err := os.Open(location)
		if err != nil {
			panic(err)
		}
		defer inFile.Close()
		data, err = ioutil.ReadAll(inFile)
		if err != nil {
			panic(err)
		}
	}()
	func() {
		outFile, err := os.Create(location)
		if err != nil {
			panic(err)
		}
		defer outFile.Close()
		strictxml.FormatIndent(bytes.NewReader(data), outFile, "", "\t")
	}()
}
