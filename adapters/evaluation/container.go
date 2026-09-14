// Package evaluation hosts generation with no evaluator files, sockets, network,
// environment or labels. It is not imported by any generation or public API host.
package evaluation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/tushardhara/dream/simulator/experiment"
)

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Container struct{ Image string }

const MaxBytes = 32 << 20

// limitedOutput does not retain an unbounded/malicious child's stdout or stderr.
type limitedOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		b.overflow = true
		return 0, fmt.Errorf("generation output budget")
	}
	return b.Buffer.Write(p)
}
func (c Container) Generate(ctx context.Context, requests []experiment.Request) ([]experiment.Projection, error) {
	if !imageID.MatchString(c.Image) || len(requests) < 1 || len(requests) > 8192 {
		return nil, fmt.Errorf("invalid isolated generation configuration")
	}
	raw, e := json.Marshal(requests)
	if e != nil || len(raw) > MaxBytes {
		return nil, fmt.Errorf("generation input budget")
	}
	output, e := c.execute(ctx, raw, "")
	if e != nil {
		return nil, e
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	var result []experiment.Projection
	if e = decoder.Decode(&result); e != nil {
		return nil, fmt.Errorf("invalid isolated generation result")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("trailing generation result")
	}
	return result, nil
}

// execute accepts only two fixed entrypoint modes. A caller cannot configure an
// arbitrary child command, mount, environment or network capability.
func (c Container) execute(ctx context.Context, raw []byte, mode string) ([]byte, error) {
	if !imageID.MatchString(c.Image) || len(raw) > MaxBytes || (mode != "" && mode != "--relationship-probe") {
		return nil, fmt.Errorf("invalid isolated generation mode")
	}
	var e error
	var nonce [12]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return nil, e
	}
	name := "dream-eval-" + hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Fixed flags and digest-only image: no mount, env passthrough, remote pull,
	// daemon socket, elevated capability or policy-configurable command argument.
	args := []string{"run", "--rm", "--pull=never", "--name", name, "--label", "dream.disposable=true", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory=256m", "--cpus=1", "--pids-limit=32", "--user=65532:65532", "--entrypoint=/hws-generate", "-i", c.Image}
	if mode != "" {
		args = append(args, mode)
	}
	// Test ownership labels are metadata only, never passed into the child.
	if run := os.Getenv("DREAM_TEST_RUN"); regexp.MustCompile(`^(codex|claude)-[0-9a-f]{32}$`).MatchString(run) {
		args = append(args[:1], append([]string{"--label", "dream.test.run=" + run, "--label", "dream.test.role=" + os.Getenv("DREAM_TEST_ROLE")}, args[1:]...)...)
	}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdin = bytes.NewReader(raw)
	var out limitedOutput
	command.Stdout = &out
	command.Stderr = io.Discard
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = exec.CommandContext(cleanup, "docker", "rm", "--force", name).Run()
	}()
	if e = command.Run(); e != nil || out.overflow {
		return nil, fmt.Errorf("isolated generation failed (no child output disclosed)")
	}
	return append([]byte(nil), out.Bytes()...), nil
}

func (c Container) ProbeRelationships(ctx context.Context, request experiment.RelationshipProbeRequest) (experiment.RelationshipProbe, error) {
	var result experiment.RelationshipProbe
	if e := request.Validate(); e != nil {
		return result, e
	}
	raw, e := json.Marshal(request)
	if e != nil {
		return result, e
	}
	output, e := c.execute(ctx, raw, "--relationship-probe")
	if e != nil {
		return result, e
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&result); e != nil {
		return result, fmt.Errorf("invalid isolated relationship evidence")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return result, fmt.Errorf("trailing relationship evidence")
	}
	return result, result.Validate(request)
}
