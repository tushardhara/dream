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
	decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
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
