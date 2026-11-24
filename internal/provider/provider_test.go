package provider

import (
	"context"
	"testing"
)

func TestSVIDSourcePrefersEnvironment(t *testing.T) {
	t.Setenv(iamLoginSVIDEnv, "token-from-env")
	// A socket that does not exist: the environment must win without touching it.
	got, err := svidSource("unix:///nonexistent.sock", iamLoginAudience, iamLoginSVIDEnv)(context.Background())
	if err != nil || got != "token-from-env" {
		t.Fatalf("got %q, %v", got, err)
	}
}
