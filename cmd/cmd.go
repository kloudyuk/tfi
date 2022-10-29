package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/kloudyuk/tfi/config"
	"github.com/kloudyuk/tfi/util"
	flag "github.com/spf13/pflag"
	"github.com/zclconf/go-cty/cty"
)

// Flags
var (
	help       bool
	noInit     bool
	region     string
	varFileDir string
)

func init() {
	flag.Usage = Usage
	flag.BoolVarP(&help, "help", "h", false, "Show help")
	flag.BoolVarP(&noInit, "no-init", "n", false, "Generate tfvars but don't run terraform init")
	flag.StringVarP(&region, "region", "r", "eu-west-1", "AWS region")
	flag.StringVar(&varFileDir, "tfvars-dir", "tfvars", "Directiry containing tfvars files")
	flag.Parse()
}

func Usage() {
	fmt.Printf("USAGE: %s NAME [FLAGS]\n\n", os.Args[0])
	fmt.Println("NAME: The name of a .tfvars file in --tfvars-dir (you can omit the .tfvars suffix)")
	fmt.Println()
	fmt.Println("FLAGS:")
	flag.PrintDefaults()
	fmt.Println()
}

func Execute() error {

	if help {
		flag.Usage()
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
		return fmt.Errorf("missing required arg: NAME")
	}

	varFile := args[0]
	if !strings.HasSuffix(varFile, ".tfvars") {
		varFile = fmt.Sprintf("%s.tfvars", varFile)
	}
	varFile = path.Join(varFileDir, varFile)
	cfg, err := config.New(varFile)
	if err != nil {
		return err
	}

	vars := cfg.Body()

	// Set env
	env := cfg.Name
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

	project, err := util.GitlabProject()
	if err != nil {
		return err
	}

	// Set session_name
	sessionName := strings.ToLower(strings.Join([]string{
		"terraform",
		project.Path,
		env,
		util.Username(),
	}, "-"))
	vars.SetAttributeValue("session_name", cty.StringVal(sessionName))

	sourceVars, err := util.Variables()
	if err != nil {
		return err
	}
	gitlabVars, err := util.GitlabVars()
	if err != nil {
		return err
	}
	for name, value := range gitlabVars {
		if sourceVars.Has(name) {
			if vars.GetAttribute(name) == nil {
				vars.SetAttributeValue(name, cty.StringVal(value))
			}
		}
	}

	// Write tfvars
	generatedVars := "tfi.auto.tfvars"
	if err := os.WriteFile(generatedVars, hclwrite.Format(cfg.Bytes()), os.ModePerm); err != nil {
		return err
	}

	if err := util.EnsureS3Backend(); err != nil {
		return err
	}

	// Leave if skipping init
	if noInit {
		return nil
	}

	// Set backend config values
	bucket := fmt.Sprintf("%s-%s-remote-state", env, region)
	key := fmt.Sprintf("%s/terraform.tfstate", project.Path)
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
