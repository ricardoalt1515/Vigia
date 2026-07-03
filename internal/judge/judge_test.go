package judge_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/ricardoalt1515/vigia/internal/judge"
)

// TestJudgeInterfaceSignatureAcceptsCtxAndReturnsError covers *Judge
// interface signature accepts ctx and returns an error*: Evaluate must
// accept context.Context first and return (JudgeResult, error), distinct
// from detection.Detector.Evaluate(in Interaction) Result.
func TestJudgeInterfaceSignatureAcceptsCtxAndReturnsError(t *testing.T) {
	judgeType := reflect.TypeOf((*judge.Judge)(nil)).Elem()

	method, ok := judgeType.MethodByName("Evaluate")
	if !ok {
		t.Fatal("judge.Judge has no Evaluate method")
	}

	// Method.Type on an interface method does not include the receiver.
	if method.Type.NumIn() != 2 {
		t.Fatalf("Evaluate has %d input params, want 2 (ctx, JudgeInput)", method.Type.NumIn())
	}
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !method.Type.In(0).Implements(ctxType) && method.Type.In(0) != ctxType {
		t.Fatalf("Evaluate's first param = %v, want context.Context", method.Type.In(0))
	}
	if method.Type.In(1) != reflect.TypeOf(judge.JudgeInput{}) {
		t.Fatalf("Evaluate's second param = %v, want judge.JudgeInput", method.Type.In(1))
	}

	if method.Type.NumOut() != 2 {
		t.Fatalf("Evaluate has %d return values, want 2 (JudgeResult, error)", method.Type.NumOut())
	}
	if method.Type.Out(0) != reflect.TypeOf(judge.JudgeResult{}) {
		t.Fatalf("Evaluate's first return = %v, want judge.JudgeResult", method.Type.Out(0))
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if method.Type.Out(1) != errType {
		t.Fatalf("Evaluate's second return = %v, want error", method.Type.Out(1))
	}
}

// TestNamedJudgePairsCodeAndJudge is a minimal compile-time-shaped check
// that NamedJudge carries a stable Code alongside a Judge implementation.
func TestNamedJudgePairsCodeAndJudge(t *testing.T) {
	nj := judge.NamedJudge{Code: "MX-REDECO-05", Judge: stubJudge{}}
	if nj.Code != "MX-REDECO-05" {
		t.Fatalf("Code = %q, want MX-REDECO-05", nj.Code)
	}
	if nj.Judge == nil {
		t.Fatal("Judge is nil")
	}
}

type stubJudge struct{}

func (stubJudge) Evaluate(_ context.Context, _ judge.JudgeInput) (judge.JudgeResult, error) {
	return judge.JudgeResult{}, nil
}

var _ judge.Judge = stubJudge{}
