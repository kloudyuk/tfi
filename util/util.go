package util

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

var variables tfVariables

type tfVariables []string

func (vars tfVariables) Has(name string) bool {
	for _, v := range vars {
		if v == name {
			return true
		}
	}
	return false
}

func Username() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u.Username
}

func AccountID(region, key string) (string, error) {
	fmt.Fprintf(os.Stderr, "Getting AWS account ID for %s\n", key)
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return "", err
	}
	svc := ssm.NewFromConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	param, err := svc.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String("account_map_json"),
	})
	if err != nil {
		return "", err
	}
	var accountMap map[string]string
	if err := json.Unmarshal([]byte(*param.Parameter.Value), &accountMap); err != nil {
		return "", err
	}
	id, ok := accountMap[key]
	if !ok {
		return "", fmt.Errorf("account ID not found for env: %s", key)
	}
	return id, nil
}

func Variables() (tfVariables, error) {
	fmt.Fprintln(os.Stderr, "Getting terraform variables")
	if variables != nil {
		return variables, nil
	}
	variables = tfVariables{}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	err = WalkHCL(cwd, func(path string, hclf *hclwrite.File) error {
		for _, block := range hclf.Body().Blocks() {
			if block.Type() == "variable" {
				variables = append(variables, block.Labels()[0])
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return variables, nil
}

func WalkHCL(root string, fn func(path string, hclf *hclwrite.File) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			return fs.SkipDir
		}
		if filepath.Ext(path) != ".tf" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hclf, diag := hclwrite.ParseConfig(b, path, hcl.InitialPos)
		if diag.HasErrors() {
			return fmt.Errorf("failed to parse %s: %s", path, diag.Error())
		}
		return fn(path, hclf)
	})
}
