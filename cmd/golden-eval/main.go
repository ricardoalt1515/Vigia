package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ricardoalt1515/vigia/internal/goldeneval"
)

type evaluator func(context.Context, goldeneval.Options) (goldeneval.Result, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, goldeneval.Run))
}

func run(args []string, stdout, stderr io.Writer, eval evaluator) int {
	flags := flag.NewFlagSet("golden-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	threshold := flags.Float64("threshold", goldeneval.DefaultThreshold, "minimum required per-rule chance-corrected agreement, from -1.0 to 1.0")
	expectedJudgeModelID := flags.String("expected-judge-model-id", "", "expected judge model id; mismatch fails the gate and requires drift re-evaluation")
	expectedRubricVersion := flags.String("expected-rubric-version", "", "expected rubric version; mismatch fails the gate and requires drift re-evaluation")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	result, err := eval(context.Background(), goldeneval.Options{
		Threshold:             *threshold,
		ExpectedJudgeModelID:  *expectedJudgeModelID,
		ExpectedRubricVersion: *expectedRubricVersion,
	})
	if err != nil && !errors.Is(err, goldeneval.ErrBelowThreshold) && !errors.Is(err, goldeneval.ErrDriftReevaluationRequired) {
		fmt.Fprintln(stderr, err)
		return 2
	}

	printResult(stdout, result)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func printResult(w io.Writer, result goldeneval.Result) {
	fmt.Fprintf(w, "golden-set agreement %.4f (%d/%d matched) model=%s rubric=%s\n", result.Agreement, result.Matched, result.Total, result.JudgeModelID, result.RubricVersion)
	for _, rule := range result.SortedRuleResults() {
		fmt.Fprintf(w, "rule=%s agreement=%.4f chance_corrected=%.4f matched=%d/%d labels={pass:%t hard_block:%t}\n", rule.RuleCode, rule.Agreement, rule.ChanceCorrectedAgreement, rule.Matched, rule.Total, rule.HasPass, rule.HasHardBlock)
	}
	for _, mismatch := range result.Mismatches {
		fmt.Fprintf(w, "mismatch case=%s rule=%s expected=%s actual=%s\n", mismatch.CaseID, mismatch.RuleCode, mismatch.Expected, mismatch.Actual)
	}
}
