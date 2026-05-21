// Package auth handles user signup against AWS Cognito.
package auth

import (
	"context"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// SignUpInput captures the fields needed to register a Cognito user.
type SignUpInput struct {
	Email          string
	FullName       string
	Password       string
	EnterpriseName string
}

// SignUp registers a new user in the configured Cognito user pool. It mirrors
// the Python mass_deployment account_creation.create_cognito_user call,
// setting email, custom:fullname, and optional custom:organization attributes.
func SignUp(appClientID, region string, in SignUpInput) error {
	if appClientID == "" {
		return fmt.Errorf("cognito app_client_id is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	client := cognitoidentityprovider.NewFromConfig(cfg)

	attrs := []types.AttributeType{
		{Name: ptr("email"), Value: ptr(in.Email)},
		{Name: ptr("custom:fullname"), Value: ptr(in.FullName)},
	}
	if in.EnterpriseName != "" {
		attrs = append(attrs, types.AttributeType{
			Name:  ptr("custom:organization"),
			Value: ptr(in.EnterpriseName),
		})
	}

	_, err = client.SignUp(ctx, &cognitoidentityprovider.SignUpInput{
		ClientId:       &appClientID,
		Username:       &in.Email,
		Password:       &in.Password,
		UserAttributes: attrs,
	})
	if err != nil {
		return fmt.Errorf("cognito sign-up: %w", err)
	}
	return nil
}

func ptr(s string) *string { return &s }
