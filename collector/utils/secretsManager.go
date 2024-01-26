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
	logger.Info().Msg("Initializing AWS service resource.")
	region := os.Getenv("AWS_REGION")
	//logger.Info().Msg()
	sess := session.Must(session.NewSession())
	svc := secretsmanager.New(sess, aws.NewConfig().WithRegion(region))
	return svc
}

func ListSecrets(logger zerolog.Logger) *secretsmanager.ListSecretsOutput {
	logger.Info().Msg("Retrieving Secrets")
	svc := getService(logger)
	input := &secretsmanager.ListSecretsInput{
		MaxResults: aws.Int64(100),
	}
	result, err := svc.ListSecrets(input)
	if err != nil {
		logger.Error().
			Err(errors.New(err.Error())).
			Msg("Failed to list secrets")
	}
	return result
}

func GetSecretsValue(logger zerolog.Logger, secret *string) string {
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
