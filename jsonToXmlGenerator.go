package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func getJSONByKey(value interface{}, name string) interface{} {
	nameDotIndex := strings.Index(name, ".")
	if nameDotIndex > 0 {
		if value == nil {
			return nil
		}
		if valueMap, ok := value.(map[string]interface{}); ok {
			v := getJSONByKey(valueMap[name[:nameDotIndex]], name[nameDotIndex+1:])
			if v == nil {
				v = getJSONByKey(valueMap["page1"], name)
			}
			if v == nil {
				v = getJSONByKey(valueMap["page2"], name)
			}
			return v
		}
	} else {
		if valueMap, ok := value.(map[string]interface{}); ok {
			return valueMap[name]
		}
	}
	return nil
}

func jsonToXML() {
	jsonInput := readJson()
	outFile, err := os.Create(*xmlTestDataFile)
	if err != nil {
		panic(err)
	}
	defer outFile.Close()

	var jsonValue interface{}
	if testDataInput, err := os.Open(*jsonTestDataFile); err == nil {
		json.NewDecoder(testDataInput).Decode(&jsonValue)
		testDataInput.Close()
	} else {
		log.Println("test data file can't be open")
		return
	}

	generateXMLParentNode := func(name string) AnyXML {
		result := AnyXML{XMLName: xml.Name{Local: name}}
		return result
	}
	stringsLast := func(arr []string) string {
		return arr[len(arr)-1]
	}
	elementNameFromKey := func(fromKey string) string {
		return "rd:" + stringsLast(strings.Split(fromKey, "."))
	}
	putXMLElement := func(parent *AnyXML, name string, child AnyXML) {
		tokens := strings.Split(name, ".")
		node := parent
	outerLoop:
		for _, currentName := range tokens[:len(tokens)-1] {
			currentName = "rd:" + currentName
			for i := range node.Nodes {
				n := &node.Nodes[i]
				if n.XMLName.Local == currentName {
					node = n
					continue outerLoop
				}
			}
			node.Nodes = append(node.Nodes, generateXMLParentNode(currentName))
			node = &node.Nodes[len(node.Nodes)-1]
		}
		node.Nodes = append(node.Nodes, child)
	}
	generateXMLElementOfType := func(name string, typ string, data interface{}) AnyXML {
		xmlElement := generateXMLParentNode(name)
		value := ""
		switch d := data.(type) {
		case bool:
			if d {
				value = "true"
			} else {
				value = "false"
			}
		case float64:
			if typ == "Number" {
				value = strconv.FormatFloat(d, 'f', 0, 64)
			} else {
				if strings.HasPrefix(typ, "Decimal") {
					decimalSpec := strings.TrimSpace(typ[len("Decimal"):])
					decimalSpec = decimalSpec[1 : len(decimalSpec)-1]
					minMax := strings.Split(decimalSpec, ",")
					if v, err := strconv.ParseInt(strings.TrimSpace(minMax[1]), 10, 64); err == nil {
						value = strconv.FormatFloat(d, 'f', int(v), 64)
					} else {
						panic("invalid decimal spec " + decimalSpec)
					}
				} else {
					panic("invalid float64 type " + typ)
				}
			}
		case string:
			value = d
		default:
			// if vc, err := json.Marshal(value); err != nil {
			// 	value = string(vc)
			// }
			panic(fmt.Sprint("unknown value type ", d))
		}
		xmlElement.Data = value
		return xmlElement
	}

	var transferValues func(*AnyXML, interface{}, []JsonOutput, string, string)
	transferValues = func(parent *AnyXML, src interface{}, datas []JsonOutput, fromKeyPrefix, toKeyPrefix string) {
		result := parent
		for datasIndex := 0; datasIndex < len(datas); datasIndex++ {
			data := datas[datasIndex]
			// PND52 have some different in json and xml,so ignore all grossReceipts and implements it manually
			if strings.HasPrefix(data.FromKey, "TaxFormDetail.Calculate.GrossReceiptsAndTaxComputation.GrossReceiptsBeforeDec.Detail") {
				continue
			}
			typ := data.Type
			switch data.FromKey {
			case "TaxForm.Filing.FilingNo", "TaxForm.Filing.FilingType":
				typ = "String"
			}
			if forceArrayType[data.FromKey] {
				typ = "Array"
			}
			if typ == "Object" {
				continue
			}
			if !strings.HasPrefix(data.FromKey, fromKeyPrefix) {
				break
			}
			toKey := data.ToKey[len(toKeyPrefix):]
			fromKey := data.FromKey[len(fromKeyPrefix):]
			if typ == "Array" {
				arrayData := getJSONByKey(src, toKey)
				newFromKeyPrefix, newToKeyPrefix := data.FromKey+".", data.ToKey+"."
				if arrayData != nil {
					if arrayDataAsArray, ok := arrayData.([]interface{}); ok {
						for _, dataInArray := range arrayDataAsArray {
							xmlElementInArray := generateXMLParentNode(elementNameFromKey(fromKey))
							transferValues(&xmlElementInArray, dataInArray, datas[datasIndex+1:], newFromKeyPrefix, newToKeyPrefix)
							putXMLElement(result, fromKey, xmlElementInArray)
						}
					} else {
						panic(fmt.Sprint("expect array but got ", arrayData))
					}
				} else {
					log.Println("ArrayData nil")
				}
				for datasIndex < len(datas) && strings.HasPrefix(datas[datasIndex].FromKey, data.FromKey) {
					datasIndex++
				}
				datasIndex--
			} else {
				value := getJSONByKey(src, toKey)
				if value != nil {
					xmlElement := generateXMLElementOfType(elementNameFromKey(fromKey), typ, value)
					putXMLElement(result, fromKey, xmlElement)
				}
			}
		}
	}
	rdForm := generateXMLParentNode(elementNameFromKey("RdForm"))
	rdForm.Attrs = append(rdForm.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:rd"}, Value: xmlNameSpace})

	putStaticData := func(parent *AnyXML, name string, value string) { // this function put some static data into xml test data
		putXMLElement(parent, name, generateXMLElementOfType(elementNameFromKey(name), "String", value))
	}
	putStaticData(&rdForm, "ExchangeDocumentContext.GuidelineSpecifiedDocumentContextParameter.Id", "123456")
	//putStaticData(&rdForm, "ExchangeDocument.Name", "แบบแสดงรายการภาษีเงินได้บริษัทหรือห้างหุ้นส่วนนิติบุคคล")

	transferValues(&rdForm, jsonValue, jsonInput, "", "")
	encoder := xml.NewEncoder(outFile)
	encoder.Indent("", "    ")
	encoder.Encode(rdForm)
}
