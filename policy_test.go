package ratelimit

import (
	"errors"
	"testing"
	"time"
)

func TestPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		policy  Policy
		wantErr bool
	}{
		{name: "valid", policy: Policy{Limit: 10, Window: time.Second}},
		{name: "zero limit", policy: Policy{Window: time.Second}, wantErr: true},
		{name: "negative limit", policy: Policy{Limit: -1, Window: time.Second}, wantErr: true},
		{name: "limit exceeds Lua precision", policy: Policy{Limit: maxLuaInteger + 1, Window: time.Second}, wantErr: true},
		{name: "zero window", policy: Policy{Limit: 1}, wantErr: true},
		{name: "sub-millisecond window", policy: Policy{Limit: 1, Window: time.Microsecond}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.policy.Validate()
			if test.wantErr && !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("Validate() error = %v, want ErrInvalidPolicy", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestSHA256KeyEncoder(t *testing.T) {
	t.Parallel()

	first := SHA256KeyEncoder("tenant:user")
	second := SHA256KeyEncoder("tenant:user")
	if first != second {
		t.Fatalf("encoder is not stable: %q != %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("encoded key length = %d, want 64", len(first))
	}
	if first == "tenant:user" {
		t.Fatal("encoder exposed the original identity")
	}
}
