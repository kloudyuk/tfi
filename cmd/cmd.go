package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/kloudyuk/tfi/util"
	flag "github.com/spf13/pflag"
	"github.com/zclconf/go-cty/cty"
)

const tfvarsFile = "tfi.auto.tfvars"
const backendFile = "tfi.backend.tf"

// Flags
var (
	gitVars    bool
	help       bool
	region     string
	tfInit     bool
	varFileDir string
)

// Args
var env string

func init() {
	flag.Usage = Usage
	flag.BoolVarP(&gitVars, "gitlab-vars", "g", true, "Add CI variables from Gitlab to tfvars")
	flag.BoolVarP(&help, "help", "h", false, "Show help")
	flag.BoolVarP(&tfInit, "init", "i", true, "Run terraform init")
	flag.StringVarP(&region, "region", "r", "eu-west-1", "AWS region")
	flag.StringVar(&varFileDir, "tfvars-dir", "tfvars", "Directiry containing tfvars files")
	flag.Parse()
}

func Usage() {
	fmt.Printf("tfi - terraform init wrapper\n\n")
	fmt.Printf("USAGE: %s ENV [FLAGS]\n\n", os.Args[0])
	fmt.Println("ENV:")
	fmt.Println("  The name of the environment, used to lookup the AWS account ID")
	fmt.Println("  If a .tfvars file exists in --tfvars-dir for this env, the vars will be added to the generated tfvars")
	fmt.Println()
	fmt.Println("FLAGS:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("IMPORTANT")
	fmt.Println("The Gitlab project path is retrieved from the git remote 'origin'")
	fmt.Println("This means your local directory must contain a .git folder with a configured remote named 'origin'")
	fmt.Println("The Gitlab API is used to retrieve the project details and Gitlab variables if required")
	fmt.Println("Please ensure you have set a valid Gitlab access token in the environment variable GITLAB_TOKEN")
	fmt.Println()
}

func Execute() error {

	// Show the usage if help requested
	if help {
		flag.Usage()
		os.Exit(0)
	}

	// Ensure we have a gitlab token
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITLAB_TOKEN not set or empty")
	}

	// Get remaining args after flags have been parsed
	args := flag.Args()

	// Ensure the mandatory args have been provided
	if len(args) < 1 {
		return fmt.Errorf("missing required arg: ENV")
	}
	env = args[0]

	// Get the AWS account ID
	accountID, err := util.AccountID(region, env)
	if err != nil {
		return err
	}

	// Get the Gitlab project info
	project, err := util.GitlabProject()
	if err != nil {
		return err
	}

	// Set the varFile based on the env
	varFile := fmt.Sprintf("%s.tfvars", env)
	varFile = path.Join(varFileDir, varFile)

	// Load the tfvars into a hclwrite.File, creating from scratch if necessary
	tfvars, err := util.LoadTFvars(varFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		tfvars = hclwrite.NewEmptyFile()
	}

	// Build a list of the terraform variable definitions
	// We'll use this to ensure we don't add unnecessary vars
	sourceVars, err := util.Variables()
	if err != nil {
		return err
	}

	// Get the body from the tfvars ready for appending additional vars
	vars := tfvars.Body()

	// Set env
	if sourceVars.Has("env") {
		vars.SetAttributeValue("env", cty.StringVal(env))
	}

	// Set region
	if sourceVars.Has("region") {
		vars.SetAttributeValue("region", cty.StringVal(region))
	}

	// Set role_arn
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s-%s", accountID, "gitlab-terraform-runner-assume-role", region)
	if sourceVars.Has("role_arn") {
		vars.SetAttributeValue("role_arn", cty.StringVal(roleARN))
	}

	// Set session_name
	sessionName := strings.ToLower(strings.Join([]string{
		"terraform",
		project.Path,
		env,
		util.Username(),
	}, "-"))
	if sourceVars.Has("session_name") {
		vars.SetAttributeValue("session_name", cty.StringVal(sessionName))
	}

	// If we're getting vars from Gitlab
	if gitVars {
		// Get the Gitlab vars
		gitlabVars, err := util.GitlabVars()
		if err != nil {
			return err
		}
		if len(gitlabVars) > 0 {
			for name, value := range gitlabVars {
				// Add only the vars from Gitlab which have a corresponding terraform variable
				if sourceVars.Has(name) {
					// Only add the var if we don't have a value already
					if vars.GetAttribute(name) == nil {
						vars.SetAttributeValue(name, cty.StringVal(value))
					}
				}
			}
		}
	}

	// Write tfvars
	fmt.Fprintf(os.Stderr, "Writing tfvars: %s\n", tfvarsFile)
	if err := os.WriteFile(tfvarsFile, hclwrite.Format(tfvars.Bytes()), os.ModePerm); err != nil {
		return err
	}

	// Set backend config values
	bucket := fmt.Sprintf("%s-%s-remote-state", env, region)
	key := fmt.Sprintf("%s/terraform.tfstate", project.Path)
	dynamoDBTable := fmt.Sprintf("%s-%s-remote-state-lock", env, region)

	// Create the backend config
	fmt.Fprintf(os.Stderr, "Writing backend config: %s\n", backendFile)
	backend := hclwrite.NewBlock("backend", []string{"s3"})
	backend.Body().SetAttributeValue("encrypt", cty.BoolVal(true))
	backend.Body().SetAttributeValue("region", cty.StringVal(region))
	backend.Body().SetAttributeValue("bucket", cty.StringVal(bucket))
	backend.Body().SetAttributeValue("key", cty.StringVal(key))
	backend.Body().SetAttributeValue("role_arn", cty.StringVal(roleARN))
	backend.Body().SetAttributeValue("session_name", cty.StringVal(sessionName))
	backend.Body().SetAttributeValue("dynamodb_table", cty.StringVal(dynamoDBTable))
	hcl := hclwrite.NewEmptyFile()
	hcl.Body().AppendNewBlock("terraform", nil).Body().AppendBlock(backend)
	if err := os.WriteFile(backendFile, hcl.Bytes(), os.ModePerm); err != nil {
		return err
	}

	// Leave if skipping init
	if !tfInit {
		return nil
	}

	// Create the init command
	cmd := exec.Command("terraform", "init")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// Remove the .terraform if it already exists
	if err := os.RemoveAll(".terraform"); err != nil {
		return err
	}

	// Run terraform init
	fmt.Fprintln(os.Stderr)
	return cmd.Run()

}
