package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tushardhara/dream/adapters/evaluation"
	"github.com/tushardhara/dream/evals"
)

type alignmentOptions struct{ plan, freeze, receipt, verify, revision, tree string }

func (a alignmentOptions) used() bool {
	return a.plan != "" || a.freeze != "" || a.receipt != "" || a.verify != "" || a.revision != "" || a.tree != ""
}

func evaluatorArtifact() (string, error) {
	path, e := os.Executable()
	if e != nil {
		return "", e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, (256<<20)+1))
	if e != nil || n > 256<<20 {
		return "", fmt.Errorf("evaluator executable budget")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Go embeds the source revision and dirty flag in ordinary CLI builds. Missing
// metadata is unavailable enforcement, never a fabricated source attestation.
func verifyEvaluatorRevision(want, tree string) error {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("evaluator build provenance unavailable")
	}
	revision, modified := "", ""
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" {
			modified = setting.Value
		}
	}
	if revision == "" || modified == "" {
		return fmt.Errorf("evaluator VCS provenance unavailable; use a clean source build")
	}
	if revision != want || modified != "false" {
		return fmt.Errorf("evaluator does not match the declared clean source revision")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "git", "rev-parse", want+"^{tree}").Output()
	if e != nil {
		return fmt.Errorf("source tree verification unavailable; freeze from the matching Git checkout")
	}
	if strings.TrimSpace(string(raw)) != tree {
		return fmt.Errorf("source tree differs from the compiled revision")
	}
	return nil
}

func writePrivateNew(path string, value any) error {
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil || len(raw) > 8<<20 {
		return fmt.Errorf("alignment artifact budget")
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return fmt.Errorf("alignment artifact must be a new owner-only file")
	}
	defer f.Close()
	if _, e = f.Write(append(raw, '\n')); e != nil {
		return e
	}
	return f.Sync()
}

func alignmentCommand(a alignmentOptions, image string, synthetic bool, datasetPath string, out io.Writer) error {
	if image == "" || (synthetic == (datasetPath != "")) {
		return fmt.Errorf("alignment requires --generator-image and exactly one synthetic dataset source")
	}
	var d evals.Dataset
	var seeds []uint64
	if synthetic {
		fixture, config, e := evals.SyntheticFixture()
		if e != nil {
			return e
		}
		d = fixture
		seeds = config.Seeds
	} else {
		if e := readPrivate(datasetPath, &d); e != nil {
			return e
		}
		// Explicit fixed default, frozen before scoring; never selected post hoc.
		seeds = []uint64{11, 23, 47, 89, 101, 131, 173, 199}
	}
	now := time.Now().UTC()
	if e := d.Validate(now); e != nil {
		return e
	}
	artifact, e := evaluatorArtifact()
	if e != nil {
		return e
	}
	if a.freeze != "" {
		if a.plan != "" || a.receipt != "" || a.verify != "" || a.revision == "" || a.tree == "" {
			return fmt.Errorf("freeze requires source revision/tree and no run/verification output")
		}
		if e = verifyEvaluatorRevision(a.revision, a.tree); e != nil {
			return e
		}
		p, e := evals.NewAlignmentPlan(d.Hash, a.revision, a.tree, artifact, image, seeds, now)
		if e != nil {
			return e
		}
		if e = writePrivateNew(a.freeze, p); e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]string{"frozen_plan_hash": p.Hash()})
	}
	if a.plan == "" || a.receipt == "" || a.revision != "" || a.tree != "" {
		return fmt.Errorf("run/verify requires retained plan and receipt, with no source override")
	}
	var p evals.AlignmentPlan
	if e = readPrivate(a.plan, &p); e != nil {
		return e
	}
	if p.Validate() != nil || p.GeneratorArtifact != image || p.DatasetHash != d.Hash {
		return fmt.Errorf("frozen alignment configuration mismatch")
	}
	if a.verify != "" {
		var r evals.AlignmentReport
		var receipt evals.AlignmentReceipt
		if e = readPrivate(a.verify, &r); e != nil {
			return e
		}
		if e = readPrivate(a.receipt, &receipt); e != nil {
			return e
		}
		if e = r.Verify(p, d, receipt); e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]string{"verified_alignment_hash": r.Hash})
	}
	if p.EvaluatorArtifact != artifact {
		return fmt.Errorf("evaluator binary differs from preregistration; freeze a new protocol before scoring")
	}
	r, receipt, e := evals.RunAlignment(context.Background(), d, p, evaluation.Container{Image: image}, now)
	if e != nil {
		return e
	}
	if e = writePrivateNew(a.receipt, receipt); e != nil {
		return e
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}
