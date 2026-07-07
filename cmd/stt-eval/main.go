package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ricardoalt1515/vigia/internal/stteval"
	"github.com/ricardoalt1515/vigia/internal/transcriber"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("stt-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	manifestPath := fs.String("manifest", "data/synthetic/audio/manifest.json", "synthetic fixture manifest")
	live := fs.Bool("live", false, "enable live provider evaluation; default is deterministic fake only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *live && os.Getenv("STT_EVAL_LIVE") != "1" {
		return fmt.Errorf("live STT eval requires STT_EVAL_LIVE=1")
	}

	manifest, err := stteval.LoadManifest(*manifestPath)
	if err != nil {
		return err
	}
	scripts := make(map[string]transcriber.Result, len(manifest.Fixtures))
	for _, f := range manifest.Fixtures {
		scripts[f.AudioURI] = transcriber.Result{Utterances: f.Reference}
	}
	report, err := stteval.Run(context.Background(), manifest.Fixtures, []stteval.NamedTranscriber{{Name: "fake", Transcriber: transcriber.NewFakeTranscriber(scripts, transcriber.Result{})}})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
