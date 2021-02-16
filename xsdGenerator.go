package main

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"strconv"
	"strings"
)

func createXsd(xsdFile string) {
	jsonInput := readJson()

	output, err := os.Create(xsdFile)
	if err != nil {
		panic(err)
	}
	defer output.Close()
	output.WriteString(xml.Header)
	output.WriteString(`<xs:schema
	attributeFormDefault="unqualified" elementFormDefault="qualified"
	targetNamespace="` + xmlNameSpace + `"
	xmlns:xs="http://www.w3.org/2001/XMLSchema"
	xmlns:rd="` + xmlNameSpace + `"
	xmlns:ds="http://www.w3.org/2000/09/xmldsig#">`)

	usedSimpleType := map[string]bool{}

	output.WriteString(`<xs:import namespace="http://www.w3.org/2000/09/xmldsig#" />`)
	writeSimple := func(name string, xmlType string, restrictions map[string]string) {
		if usedSimpleType[name] {
			return
		}
		usedSimpleType[name] = true
		output.WriteString(`<xs:simpleType name="` + name + `"><xs:restriction base="` + xmlType + `">` + "\n")
		for key, value := range restrictions {
			output.WriteString(`<xs:` + key + ` value="` + value + `"/>`)
		}
		output.WriteString(`</xs:restriction></xs:simpleType>` + "\n")
	}

	parentChildMap := map[string][]string{}
	ruleMap := map[string]JsonOutput{}
	parentHasChildMap := map[string]map[string]bool{}
	resolvedType := map[string]string{}
	for _, rule := range jsonInput {
		tokens := strings.Split(rule.FromKey, ".")
		for i := range tokens {
			parentKey := strings.Join(tokens[:i], ".")
			childKey := strings.Join(tokens[:i+1], ".")
			if parentHasChildMap[parentKey] == nil {
				parentHasChildMap[parentKey] = make(map[string]bool)
			}
			if parentHasChildMap[parentKey][childKey] {
				continue
			}
			parentHasChildMap[parentKey][childKey] = true
			parentChildMap[parentKey] = append(parentChildMap[parentKey], childKey)
		}
		var xmlType string
		setType := func(name string, _xmlType string, restriction map[string]string) {
			xmlType = name
			writeSimple(name, _xmlType, restriction)
		}
		switch rule.Type {
		case "Boolean":
			typeName := "booleanType"
			restrictions := make(map[string]string)
			setType(typeName, "xs:boolean", restrictions)
		case "Number":
			typeName := "numberType"
			restrictions := make(map[string]string)
			if rule.MaxLength != "" {
				restrictions["totalDigits"] = rule.MaxLength
				typeName += "Digits" + rule.MaxLength
			}
			setType(typeName, "xs:integer", restrictions)
		case "String":
			typeName := "stringType"
			restrictions := map[string]string{}
			if rule.MaxLength != "" {
				restrictions["maxLength"] = rule.MaxLength
				typeName += "Max" + rule.MaxLength
			}
			setType(typeName, "xs:string", restrictions)
		case "Date":
			setType("dateType", "xs:date", nil)
		case "Array":
		case "Object":
		default:
			switch {
			case strings.HasPrefix(rule.Type, "Decimal"):
				a1 := strings.Index(rule.Type, "(")
				a2 := strings.Index(rule.Type, ",") // decimal delimiter is comma
				if a2 < 0 {
					a2 = strings.Index(rule.Type, ".") // decimal delimiter maybe dot
				}
				a3 := strings.Index(rule.Type, ")")
				if a1 < 0 || a2 < 0 || a3 < 0 {
					panic("unknown decimal type" + rule.Type)
				}
				precision := strings.TrimSpace(rule.Type[a1+1 : a2])
				scale := strings.TrimSpace(rule.Type[a2+1 : a3])
				typeName := "decimalType" + precision + "fraction" + scale
				restrictions := make(map[string]string)
				restrictions["totalDigits"] = precision
				restrictions["fractionDigits"] = scale
				setType(typeName, "xs:decimal", restrictions)
			default:
				panic("unknown type " + rule.Type)
			}
		}
		ruleMap[rule.FromKey] = rule
		if xmlType != "" {
			resolvedType[rule.FromKey] = xmlType
		}
	}

	usedComplexType := map[string]string{}
	generateTypeKey := func() string {
		return "dynamicType" + strconv.FormatInt(int64(len(usedComplexType)+1), 10)
	}
	var printOutType func(string) string
	printOutType = func(_typeKey string) string {
		if str, ok := resolvedType[_typeKey]; ok {
			return str
		}
		resolvedType[_typeKey] = func() string {
			type ChildType struct {
				Name      string
				Type      string
				MinOccurs string
				MaxOccurs string
			}
			var childs []ChildType
			for _, childKey := range parentChildMap[_typeKey] {
				childType := printOutType(childKey)
				tokens := strings.Split(childKey, ".")
				childName := tokens[len(tokens)-1]
				minMax := ruleMap[childKey].Multiple
				var minMaxTokens []string
				if strings.TrimSpace(minMax) == "" {
					minMaxTokens = []string{"1", "1"}
				} else {
					if minMax[0] != '[' || minMax[len(minMax)-1] != ']' {
						panic("weird min max" + minMax + " " + ruleMap[childKey].FromKey)
					}
					minMax = minMax[1 : len(minMax)-1]
					minMaxTokens = strings.Split(minMax, "…")
				}
				if len(minMaxTokens) != 2 {
					panic("weird Min Max " + minMax)
				}
				switch minMaxTokens[1] {
				case "*", "n":
					minMaxTokens[1] = "unbounded"
				}
				childs = append(childs, ChildType{
					Name:      childName,
					Type:      childType,
					MinOccurs: minMaxTokens[0],
					MaxOccurs: minMaxTokens[1],
				})
			}
			typeValue, err := json.MarshalIndent(childs, "", "")
			if err != nil {
				panic(err)
			}
			if typeKey, ok := usedComplexType[string(typeValue)]; ok {
				return typeKey
			}
			typeKey := func() string {
				typeKey := generateTypeKey()
				output.WriteString(`<xs:complexType name="` + typeKey + `"><xs:sequence>`)
				for _, child := range childs {
					output.WriteString(`<xs:element name="` + child.Name + `" type="rd:` + child.Type + `"`)
					if child.MinOccurs != "1" {
						output.WriteString(` minOccurs="` + child.MinOccurs + `"`)
					}
					if child.MaxOccurs != "1" {
						output.WriteString(` maxOccurs="` + child.MaxOccurs + `"`)
					}
					output.WriteString("/>")
				}
				if _typeKey == "" {
					output.WriteString(`<xs:element ref="ds:Signature" minOccurs="0" />`)
				}
				output.WriteString(`</xs:sequence></xs:complexType>`)
				return typeKey
			}()
			usedComplexType[string(typeValue)] = typeKey
			return typeKey
		}()
		return resolvedType[_typeKey]
	}
	rootType := printOutType("")
	output.WriteString(`<xs:element name="RdForm" type="rd:` + rootType + `"/>`)
	output.WriteString(`</xs:schema>`)
}
