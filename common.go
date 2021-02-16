package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

var internalJSONBuffer []byte

func readJson() []JsonOutput {
	var reader io.Reader
	if *tempJSONFile != "" {
		file, err := os.Open(*tempJSONFile)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		reader = file
	} else {
		reader = bytes.NewReader(internalJSONBuffer)
	}
	var jsonInput []JsonOutput
	err := json.NewDecoder(reader).Decode(&jsonInput)
	if err != nil {
		panic(err)
	}
	return jsonInput
}

func writeJson(jsonOutput []JsonOutput) {
	var writer io.Writer
	if *tempJSONFile != "" {
		file, err := os.Create(*tempJSONFile)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		writer = file
	} else {
		var buffer bytes.Buffer
		writer = &buffer
		defer func() {
			internalJSONBuffer = buffer.Bytes()
		}()
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "    ")
	err := encoder.Encode(jsonOutput)
	if err != nil {
		panic(err)
	}
}
