package util

import (
	"context"
	"encoding/json"
	"os/user"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func Username() string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	return u.Username
}

func AccountID(region, key string) (string, error) {
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
	return accountMap[key], nil
}
