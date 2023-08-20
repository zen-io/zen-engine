package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	hclconv "github.com/tmccombs/hcl2json/convert"
)

func ReadHclFile(path string) (map[string][]map[string]interface{}, error) {
	content, err := os.ReadFile(path)
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

func DeepCopyStringMap(m map[string]string) map[string]string {
	res := make(map[string]string)
	for k, v := range m {
		res[k] = v
	}

	return res
}
