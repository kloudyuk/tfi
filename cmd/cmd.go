package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/kloudyuk/tfi/config"
	"github.com/kloudyuk/tfi/util"
	flag "github.com/spf13/pflag"
	"github.com/zclconf/go-cty/cty"
)

// Flags
var (
	help     bool
	region   string
	skipInit bool
	varFile  string
)

func init() {
	flag.Usage = Usage
	flag.BoolVarP(&help, "help", "h", false, "Show help")
	flag.StringVarP(&region, "region", "r", "eu-west-1", "AWS region")
	flag.BoolVarP(&skipInit, "skip", "s", false, "Skip running the terraform init")
	flag.StringVarP(&varFile, "var-file", "v", "", "Path to .tfvars file")
	flag.Parse()
}

func Usage() {
	fmt.Printf("Usage:\n  %s [FLAGS]\n", os.Args[0])
	fmt.Println("FLAGS:")
	flag.PrintDefaults()
}

func Execute() error {

	if help {
		flag.Usage()
		os.Exit(0)
	}

	cfg, err := config.New(varFile)
	if err != nil {
		return err
	}

	vars := cfg.Body()

	// Set env
	var env = cfg.Name
	vars.SetAttributeValue("env", cty.StringVal(env))

	// Set region
	vars.SetAttributeValue("region", cty.StringVal(region))

	// Set role_arn
	accountID, err := util.AccountID(region, env)
	if err != nil {
		return err
	}
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s-%s", accountID, "gitlab-terraform-runner-assume-role", region)
	vars.SetAttributeValue("role_arn", cty.StringVal(roleARN))

	// Set session_name
	projectName, err := util.GitProjectName()
	if err != nil {
		return err
	}
	sessionName := strings.Join([]string{
		"terraform",
		projectName,
		env,
		util.Username(),
	}, "-")
	vars.SetAttributeValue("session_name", cty.StringVal(sessionName))

	// Write tfvars
	generatedVars := "tfi.auto.tfvars"
	if err := os.WriteFile(generatedVars, hclwrite.Format(cfg.Bytes()), os.ModePerm); err != nil {
		return err
	}

	// Leave if skipping init
	if skipInit {
		return nil
	}

	// Set backend config values
	bucket := fmt.Sprintf("%s-%s-remote-state", env, region)
	key := fmt.Sprintf("%s/terraform.tfstate", projectName)
	dynamoDBTable := fmt.Sprintf("%s-%s-remote-state-lock", env, region)

	// Run terraform init
	cmd := exec.Command(
		"terraform", "init",
		"-backend-config=encrypt=true",
		fmt.Sprintf("-backend-config=region=%s", region),
		fmt.Sprintf("-backend-config=bucket=%s", bucket),
		fmt.Sprintf("-backend-config=key=%s", key),
		fmt.Sprintf("-backend-config=role_arn=%s", roleARN),
		fmt.Sprintf("-backend-config=session_name=%s", sessionName),
		fmt.Sprintf("-backend-config=dynamodb_table=%s", dynamoDBTable),
	)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := os.RemoveAll(".terraform"); err != nil {
		return err
	}

	return cmd.Run()

}