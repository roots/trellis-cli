package engine_test

import (
	"context"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/engine"
)

func BenchmarkCompile(b *testing.B) {
	for b.Loop() {
		compileEnv(b, "full", "production", nil)
	}
}

func BenchmarkObservePlanConverged(b *testing.B) {
	m := compileEnv(b, "full", "production", nil)
	h := newFake()
	converge(b, h, m)
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		obs, err := engine.Observe(ctx, h, m)
		if err != nil {
			b.Fatal(err)
		}
		if p := engine.Compute(m, obs); !p.Empty() {
			b.Fatal("not empty")
		}
	}
}
