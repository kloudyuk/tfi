package util

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

func LoadTFvars(varFile string) (*hclwrite.File, error) {
	fmt.Printf("Loading tfvars: %s\n", varFile)
	b, err := os.ReadFile(varFile)
	if err != nil {
		return nil, err
	}
	// Ensure the bytes end with a newline
	if !strings.HasSuffix(string(b), "\n") {
		b = append(b, []byte("\n")...)
	}
	hclf, diag := hclwrite.ParseConfig(b, varFile, hcl.InitialPos)
	if diag.HasErrors() {
		return nil, fmt.Errorf(diag.Error())
	}
	return hclf, nil
}
