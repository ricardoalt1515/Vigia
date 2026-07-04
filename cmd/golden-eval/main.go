package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/ricardoalt1515/vigia/internal/goldeneval"
)

func main() {
	threshold := flag.Float64("threshold", goldeneval.DefaultThreshold, "minimum required golden-set agreement, from 0.0 to 1.0")
	flag.Parse()

	result, err := goldeneval.Run(context.Background(), goldeneval.Options{Threshold: *threshold})
	if err != nil && !errors.Is(err, goldeneval.ErrBelowThreshold) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Fprintf(os.Stdout, "golden-set agreement %.4f (%d/%d matched)\n", result.Agreement, result.Matched, result.Total)
	for _, mismatch := range result.Mismatches {
		fmt.Fprintf(os.Stdout, "mismatch case=%s rule=%s expected=%s actual=%s\n", mismatch.CaseID, mismatch.RuleCode, mismatch.Expected, mismatch.Actual)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
