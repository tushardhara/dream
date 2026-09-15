package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/tushardhara/dream/adapters/evaluation"
	"github.com/tushardhara/dream/evals"
)

// Owner-readable files only. The generator container never receives these paths
// or a mount containing them. Synthetic data authorization remains enforced by
// Dataset.Validate; a mode check is not authority to import real human records.
func readPrivate(path string, value any) error {
	file, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return fmt.Errorf("cannot open evaluator file")
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("evaluator file must be regular and owner-only")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("evaluator file owner mismatch")
	}
	raw, e := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if e != nil || len(raw) > 8<<20 {
		return fmt.Errorf("evaluator file budget")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(value); e != nil {
		return fmt.Errorf("invalid evaluator file")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("trailing evaluator data")
	}
	return nil
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("hws-eval", flag.ContinueOnError)
	flags.SetOutput(out)
	image := flags.String("generator-image", "", "required local sha256 image ID; no pulling")
	synthetic := flags.Bool("synthetic", false, "use versioned synthetic format fixture")
	dataset := flags.String("dataset", "", "owner-only synthetic dataset JSON")
	config := flags.String("config", "", "owner-only frozen evaluator config JSON")
	alignmentPlan := flags.String("alignment-plan", "", "owner-only frozen complete HWS registry experiment plan")
	freezeAlignment := flags.String("freeze-alignment-plan", "", "write a new owner-only preregistration; performs no generation")
	alignmentReceipt := flags.String("alignment-receipt", "", "new execution receipt for run; retained receipt for verification")
	verifyAlignment := flags.String("verify-alignment-report", "", "owner-only complete registry report to verify without generation")
	sourceRevision := flags.String("source-revision", "", "exact source commit for alignment preregistration")
	sourceTree := flags.String("source-tree", "", "exact source tree for alignment preregistration")
	studyPlan := flags.String("study-plan", "", "owner-only frozen 30-real-day study plan JSON")
	study := flags.String("study-action", "", "explicit register/status/day/abandon-expired; no scheduler or live provider")
	development := flags.Bool("development", false, "explicit disposable/local study database")
	uplift := flags.Bool("uplift", false, "emit the bounded synthetic matched-arm uplift summary; no generator, provider or network")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if *uplift {
		if flags.NArg() != 0 || *image != "" || *synthetic || *dataset != "" || *config != "" || *studyPlan != "" || *study != "" || *development {
			return fmt.Errorf("--uplift takes no other options")
		}
		summary, e := evals.SummariseUplift(evals.UpliftFixture())
		if e != nil {
			return e
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}
	alignment := alignmentOptions{*alignmentPlan, *freezeAlignment, *alignmentReceipt, *verifyAlignment, *sourceRevision, *sourceTree}
	if alignment.used() {
		if *studyPlan != "" || *study != "" || *development || *config != "" || flags.NArg() != 0 {
			return fmt.Errorf("ambiguous alignment/study/evaluation options")
		}
		return alignmentCommand(alignment, *image, *synthetic, *dataset, out)
	}
	if *studyPlan != "" || *study != "" {
		if flags.NArg() != 0 || *studyPlan == "" || *study == "" || *image != "" || *synthetic || *config != "" || (*study != "day" && *dataset != "") {
			return fmt.Errorf("ambiguous study/evaluation options")
		}
		return studyAction(*study, *studyPlan, *dataset, *development, out)
	}
	if *development {
		return fmt.Errorf("development database option requires a study action")
	}
	if flags.NArg() != 0 || *image == "" || (*synthetic && (*dataset != "" || *config != "")) || (!*synthetic && (*dataset == "" || *config == "")) {
		return fmt.Errorf("choose --synthetic or --dataset/--config and supply --generator-image")
	}
	var d evals.Dataset
	var c evals.Config
	var e error
	if *synthetic {
		d, c, e = evals.SyntheticFixture()
		if e != nil {
			return e
		}
		c.GeneratorArtifact = *image
	} else {
		if e = readPrivate(*dataset, &d); e != nil {
			return e
		}
		if e = readPrivate(*config, &c); e != nil {
			return e
		}
		if c.GeneratorArtifact != *image {
			return fmt.Errorf("image differs from frozen evaluator configuration")
		}
	}
	r, e := evals.Run(context.Background(), d, c, evaluation.Container{Image: *image}, time.Now())
	if e != nil {
		return e
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}
func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		if e == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
