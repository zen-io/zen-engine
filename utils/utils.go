package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	hclconv "github.com/tmccombs/hcl2json/convert"
)

func ReadHclFile(path string) (map[string][]map[string]interface{}, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return FromHclBytes(content, filepath.Base(path))
}

func FromHclBytes(content []byte, filename string) (map[string][]map[string]interface{}, error) {
	jsonBytes, err := hclconv.Bytes(content, filename, hclconv.Options{
		Simplify: false,
	})
	if err != nil {
		return nil, fmt.Errorf("converting from hcl %s: %w", filename, err)
	}

	var pkgBlocks map[string][]map[string]interface{}
	err = json.Unmarshal(jsonBytes, &pkgBlocks)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling blocks: %w", err)
	}

	return pkgBlocks, nil
}
