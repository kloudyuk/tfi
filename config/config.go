package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

type config struct {
	*hclwrite.File
	Name string
}

func New(varFile string) (*config, error) {
	hclf, err := load(varFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		hclf = hclwrite.NewEmptyFile()
	}
	name := "default"
	if varFile != "" {
		filename := filepath.Base(varFile)
		name = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	return &config{
		File: hclf,
		Name: name,
	}, nil
}

func load(varFile string) (*hclwrite.File, error) {
	b, err := os.ReadFile(varFile)
	if err != nil {
		return nil, err
	}
	hclf, diag := hclwrite.ParseConfig(b, varFile, hcl.InitialPos)
	if diag.HasErrors() {
		return nil, fmt.Errorf(diag.Error())
	}
	return hclf, nil
}
