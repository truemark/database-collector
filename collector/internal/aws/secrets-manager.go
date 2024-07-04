package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"os"
)

func getService() *secretsmanager.SecretsManager {
	region := os.Getenv("AWS_REGION")
	sess := session.Must(session.NewSession())
	svc := secretsmanager.New(sess, aws.NewConfig().WithRegion(region))
	return svc
}

func ListSecrets() *secretsmanager.ListSecretsOutput {
	svc := getService()
	input := &secretsmanager.ListSecretsInput{
		MaxResults: aws.Int64(100),
		Filters: []*secretsmanager.Filter{
			{
				Key:    aws.String("tag-key"),
				Values: aws.StringSlice([]string{"database-collector:enabled"}),
			},
		},
	}
	result, err := svc.ListSecrets(input)
	if err != nil {
		fmt.Println(err)
	}
	return result
}

func GetSecretsValue(secret *string) string {
	svc := getService()
	input := &secretsmanager.GetSecretValueInput{
		SecretId: secret,
	}
	result, err := svc.GetSecretValue(input)
	if err != nil {
		fmt.Println(err)
	}
	return *result.SecretString
}
