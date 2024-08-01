package aws

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-secretsmanager-caching-go/secretcache"
	"os"
)

var (
	config = secretcache.CacheConfig{
		MaxCacheSize: secretcache.DefaultMaxCacheSize + 10,
		VersionStage: secretcache.DefaultVersionStage,
		CacheItemTTL: secretcache.DefaultCacheItemTTL,
	}
	secretCache, _ = secretcache.New(func(cache *secretcache.Cache) {
		cache.CacheConfig = config
	})
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

func GetSecretsValue(secret string) string {
	var result, err = secretCache.GetSecretString(secret)
	if err != nil {
		panic(err)
	}
	return result
}
