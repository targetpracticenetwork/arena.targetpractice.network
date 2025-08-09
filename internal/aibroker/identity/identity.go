package identity

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Validator struct {
	client   kubernetes.Interface
	audience string
	enabled  bool
}

func NewValidator(audience string, enabled bool) (*Validator, error) {
	if !enabled {
		return &Validator{enabled: false}, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Validator{
		client:   client,
		audience: audience,
		enabled:  true,
	}, nil
}

func (v *Validator) ValidateRequest(r *http.Request) error {
	if !v.enabled {
		return nil
	}

	tokenHeader := r.Header.Get("X-Service-Account-Token")
	if tokenHeader == "" {
		return fmt.Errorf("missing X-Service-Account-Token header")
	}

	// Extract token from "Bearer <token>" format
	token := strings.TrimPrefix(tokenHeader, "Bearer ")
	if token == tokenHeader {
		return fmt.Errorf("X-Service-Account-Token must be in 'Bearer <token>' format")
	}

	return v.validateToken(r.Context(), token)
}

func (v *Validator) validateToken(ctx context.Context, token string) error {
	tokenReview := &authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token:     token,
			Audiences: []string{v.audience},
		},
	}

	result, err := v.client.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("token review failed: %w", err)
	}

	if !result.Status.Authenticated {
		return fmt.Errorf("token authentication failed: %s", result.Status.Error)
	}

	// Check if the token has the expected audience
	audienceFound := false
	for _, aud := range result.Status.Audiences {
		if aud == v.audience {
			audienceFound = true
			break
		}
	}

	if !audienceFound {
		return fmt.Errorf("token does not have required audience: %s", v.audience)
	}

	return nil
}