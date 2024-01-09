package utils

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/rs/zerolog"
	"os"
)

func getService(logger zerolog.Logger) *secretsmanager.SecretsManager {
	region := os.Getenv("AWS_REGION")
	//logger.Info().Msg()
	sess := session.Must(session.NewSession())
	svc := secretsmanager.New(sess, aws.NewConfig().WithRegion(region))
	return svc
}

func ListSecrets(logger zerolog.Logger) *secretsmanager.ListSecretsOutput {
	svc := getService(logger)
	input := &secretsmanager.ListSecretsInput{}
	result, err := svc.ListSecrets(input)
	if err != nil {
		logger.Error().
			Err(errors.New(err.Error())).
			Msg("Failed to list secrets")
	}
	//fmt.Println(result.SecretList)
	for i := 0; i < len(result.SecretList); i++ {
		for x := 0; x < len(result.SecretList[i].Tags); x++ {
			if *result.SecretList[i].Tags[x].Key == "database-collector:enabled" {
				if *result.SecretList[i].Tags[x].Value == "true" {
					secretValue := getSecretsValue(logger, result.SecretList[i].Name)
					logger.Debug().Msg(secretValue)
				}
			}
		}
	}
	return result
}

func getSecretsValue(logger zerolog.Logger, secret *string) string {
	svc := getService(logger)
	input := &secretsmanager.GetSecretValueInput{
		SecretId: secret,
	}
	result, err := svc.GetSecretValue(input)
	if err != nil {
		logger.Error().Msg(err.Error())
	}
	return *result.SecretString
}
