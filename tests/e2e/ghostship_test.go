//go:build e2e

// End-to-end tests for the ghostship backup daemon (#604), driven through the real
// cargoship binary against the in-process Substrate S3 emulator. These prove the
// ghostship-specific orchestration that unit tests can't: the writers/<id>/ key
// layout, a full writer-scoped round-trip (upload → restore, byte-identical), the
// writer-scoped INCREMENTAL chain (a second cycle finds the prior manifest under the
// writer prefix), and config-file multi-source runs.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/fleet"
)

// e2eS3Client builds an emulator-pointed S3 client (mirrors createBucket).
func e2eS3Client(t *testing.T) *s3.Client {
	t.Helper()
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(substrateURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
}

// uploadIDsUnder returns the sorted upload-id segments under <prefix>/uploads/ — used
// to assert the writer-isolated layout and count version cycles.
func uploadIDsUnder(t *testing.T, client *s3.Client, bucket, prefix string) []string {
	t.Helper()
	p := strings.TrimSuffix(prefix, "/") + "/uploads/"
	out, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(p),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		t.Fatalf("list uploads under %s: %v", p, err)
	}
	var ids []string
	for _, cp := range out.CommonPrefixes {
		seg := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), p), "/")
		if seg != "" {
			ids = append(ids, seg)
		}
	}
	sort.Strings(ids)
	return ids
}

// TestGhostshipRun_RoundTrip proves the writer-isolated layout and a full round-trip:
// `ghostship run --once --writer-id` writes under writers/<id>/uploads/, and restore
// returns the exact bytes.
func TestGhostshipRun_RoundTrip(t *testing.T) {
	bucket := "gs-roundtrip"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "greeting.txt"), "hello ghostship")
	if err := os.MkdirAll(filepath.Join(src, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(src, "docs", "readme.md"), "# readme\nnested content")

	runCargoship(t, "ghostship", "run", src, "s3://"+bucket+"/archives",
		"--writer-id", "lab-nas-1", "--once", "--region", "us-east-1")

	client := e2eS3Client(t)
	ids := uploadIDsUnder(t, client, bucket, "archives/writers/lab-nas-1")
	if len(ids) != 1 {
		t.Fatalf("want exactly 1 upload under archives/writers/lab-nas-1/uploads/, got %d: %v", len(ids), ids)
	}
	uploadURL := "s3://" + bucket + "/archives/writers/lab-nas-1/uploads/" + ids[0]

	restoreDir := t.TempDir()
	for _, tc := range []struct{ abs, base, want string }{
		{filepath.Join(src, "greeting.txt"), "greeting.txt", "hello ghostship"},
		{filepath.Join(src, "docs", "readme.md"), "readme.md", "# readme\nnested content"},
	} {
		runCargoship(t, "restore", uploadURL, restoreDir, "--region", "us-east-1", "--file", tc.abs)
		got, err := os.ReadFile(findFileByBase(t, restoreDir, tc.base))
		if err != nil {
			t.Fatalf("read restored %s: %v", tc.base, err)
		}
		if string(got) != tc.want {
			t.Fatalf("restored %s = %q, want %q", tc.base, got, tc.want)
		}
	}

	// #615: the cycle wrote a heartbeat under the writer prefix, and it parses.
	hb := getStatus(t, client, bucket, "archives/writers/lab-nas-1/status.json")
	if hb.WriterID != "lab-nas-1" {
		t.Errorf("heartbeat writer_id = %q, want lab-nas-1", hb.WriterID)
	}
	if !hb.Healthy() {
		t.Errorf("heartbeat should be healthy after a clean cycle: %+v", hb.Sources)
	}
	if len(hb.Sources) != 1 || hb.Sources[0].Files == 0 {
		t.Errorf("heartbeat should record the backed-up source with a file count: %+v", hb.Sources)
	}
}

// getStatus fetches and parses a writer heartbeat object (#615).
func getStatus(t *testing.T, client *s3.Client, bucket, key string) fleet.WriterStatus {
	t.Helper()
	obj, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(obj.Body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	var st fleet.WriterStatus
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("parse %s: %v", key, err)
	}
	return st
}

// TestFleetStatus_ListsWriter proves the control-side read path: after a ghostship
// cycle writes a heartbeat, `cargoship fleet status` lists that writer as healthy.
func TestFleetStatus_ListsWriter(t *testing.T) {
	bucket := "gs-fleet-status"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "hello")

	runCargoship(t, "ghostship", "run", src, "s3://"+bucket+"/nas",
		"--writer-id", "box-1", "--once", "--region", "us-east-1")

	out := runCargoship(t, "fleet", "status", "s3://"+bucket+"/nas", "--region", "us-east-1")
	if !strings.Contains(out, "box-1") {
		t.Fatalf("fleet status should list writer box-1, got:\n%s", out)
	}
	if !strings.Contains(out, "yes") {
		t.Fatalf("fleet status should show box-1 as healthy, got:\n%s", out)
	}

	// --json emits the parsed heartbeat. CombinedOutput may prepend a "Using config
	// file" notice on stderr, so parse from the first JSON array bracket.
	jsonOut := runCargoship(t, "fleet", "status", "s3://"+bucket+"/nas", "--region", "us-east-1", "--json")
	if i := strings.IndexByte(jsonOut, '['); i >= 0 {
		jsonOut = jsonOut[i:]
	}
	var statuses []fleet.WriterStatus
	if err := json.Unmarshal([]byte(jsonOut), &statuses); err != nil {
		t.Fatalf("fleet status --json is not valid JSON: %v\n%s", err, jsonOut)
	}
	if len(statuses) != 1 || statuses[0].WriterID != "box-1" {
		t.Fatalf("fleet status --json = %+v, want one writer box-1", statuses)
	}
}

// TestFleetMonitor_DetectsStale proves `fleet monitor --once` reads heartbeats and
// flags a writer past the freshness threshold. A near-zero threshold makes the
// just-written heartbeat count as stale, so the check runs without waiting.
func TestFleetMonitor_DetectsStale(t *testing.T) {
	bucket := "gs-fleet-monitor"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "hello")

	runCargoship(t, "ghostship", "run", src, "s3://"+bucket+"/nas",
		"--writer-id", "mon-1", "--once", "--region", "us-east-1")

	out := runCargoship(t, "fleet", "monitor", "s3://"+bucket+"/nas",
		"--once", "--threshold", "1ns", "--interval", "1h", "--region", "us-east-1")
	if !strings.Contains(out, "writer stale") || !strings.Contains(out, "mon-1") {
		t.Fatalf("fleet monitor should flag mon-1 as stale, got:\n%s", out)
	}
}

// TestFleetLockStatus_Runs proves the immutability posture audit runs end-to-end and
// reports all three checks. The emulator may not implement versioning/Object Lock/
// lifecycle, so those checks can come back UNKNOWN — the point is the command runs
// read-only and renders every check rather than any specific verdict.
func TestFleetLockStatus_Runs(t *testing.T) {
	bucket := "gs-fleet-lock"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	out := runCargoship(t, "fleet", "lock-status", "s3://"+bucket+"/nas", "--region", "us-east-1")
	for _, want := range []string{"versioning", "object-lock", "lifecycle-mpu", "Overall:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("fleet lock-status output missing %q, got:\n%s", want, out)
		}
	}
}

// TestGhostshipRun_IncrementalChain proves the writer-scoped incremental chain: a
// no-change cycle uploads nothing, and a subsequent change produces a second version
// under the same writer prefix (i.e. the prior manifest is found under writers/<id>/).
func TestGhostshipRun_IncrementalChain(t *testing.T) {
	bucket := "gs-incremental"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "one")

	runOut := func(label string) string {
		out, err := runCargoshipAllowErr(t, "ghostship", "run", src, "s3://"+bucket+"/data", "--writer-id", "inc-1", "--once", "--region", "us-east-1")
		if err != nil {
			t.Fatalf("%s: %v\n%s", label, err, out)
		}
		return out
	}

	client := e2eS3Client(t)
	count := func() int { return len(uploadIDsUnder(t, client, bucket, "data/writers/inc-1")) }

	// First cycle has no prior manifest → full sync, one version.
	if out := runOut("first"); !strings.Contains(out, "sync_type=full") {
		t.Fatalf("first cycle should be a full sync, got:\n%s", out)
	}
	if got := count(); got != 1 {
		t.Fatalf("first cycle should create 1 upload, got %d", got)
	}

	// A no-change cycle must dedup — detect no changes and upload NOTHING new (#624).
	if out := runOut("nochange"); !strings.Contains(out, "no changes") {
		t.Fatalf("no-change cycle should detect no changes and upload nothing (#624), got:\n%s", out)
	}
	if got := count(); got != 1 {
		t.Fatalf("a no-change cycle must not create a new upload (delta dedup, #624); got %d", got)
	}

	// A real change chains to the prior manifest UNDER THE WRITER PREFIX
	// (writers/inc-1/uploads/…) → incremental, producing a second version. This is the
	// writer-scoped-chain proof: FindLatestManifestForSource resolves within the folded
	// prefix, which no unit test can verify.
	writeFile(t, filepath.Join(src, "a.txt"), "one-changed")
	if out := runOut("changed"); !strings.Contains(out, "sync_type=incremental") {
		t.Fatalf("changed cycle should chain to the prior manifest under the writer prefix (incremental), got:\n%s", out)
	}
	if got := count(); got != 2 {
		t.Fatalf("after a change want 2 uploads under data/writers/inc-1/uploads/, got %d", got)
	}
}

func putObjectBytes(t *testing.T, client *s3.Client, bucket, key string, data []byte) {
	t.Helper()
	if _, err := client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data),
	}); err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
}

func putObjectFile(t *testing.T, client *s3.Client, bucket, key, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	putObjectBytes(t, client, bucket, key, data)
}

// signAndUploadConfig keygens (into keyDir), writes+signs the config, and uploads the
// config + signature under <base>/fleet/<writerID>/. Returns the public-key path.
func signAndUploadConfig(t *testing.T, client *s3.Client, bucket, base, writerID, configYAML string) (pubPath string) {
	t.Helper()
	keyDir := t.TempDir()
	runCargoship(t, "ghostship", "config-keygen", "--out", keyDir)
	priv := filepath.Join(keyDir, "config-signing-private.pem")
	pubPath = filepath.Join(keyDir, "config-signing-public.pem")

	box := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, box, configYAML)
	runCargoship(t, "ghostship", "sign-config", box, "--key", priv)

	cp := base + "/fleet/" + writerID
	putObjectFile(t, client, bucket, cp+"/config.yaml", box)
	putObjectFile(t, client, bucket, cp+"/config.yaml.sig", box+".sig")
	return pubPath
}

// TestGhostshipRun_ConfigPull proves signed config-over-S3 (#614): the daemon pulls its
// config from the control prefix, verifies the ed25519 signature, and runs it — data
// lands under the data prefix and the heartbeat reports the config version.
func TestGhostshipRun_ConfigPull(t *testing.T) {
	bucket := "gs-config-pull"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "hello pull")

	client := e2eS3Client(t)
	cfgYAML := fmt.Sprintf("id: pull-box\nwriter_id: pull-1\nversion: 5\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src)
	pub := signAndUploadConfig(t, client, bucket, "nas", "pull-1", cfgYAML)

	runCargoship(t, "ghostship", "run", "--config-url", "s3://"+bucket+"/nas",
		"--public-key", pub, "--writer-id", "pull-1", "--once", "--region", "us-east-1")

	// Data lands under the data prefix (bucket-root writers/<id>/, from the config).
	if ids := uploadIDsUnder(t, client, bucket, "writers/pull-1"); len(ids) != 1 {
		t.Fatalf("want 1 upload under writers/pull-1/uploads/, got %d: %v", len(ids), ids)
	}
	if hb := getStatus(t, client, bucket, "writers/pull-1/status.json"); hb.ConfigVersion != 5 {
		t.Fatalf("heartbeat config_version = %d, want 5", hb.ConfigVersion)
	}
}

// TestGhostshipRun_ConfigPull_RejectsTampered proves the daemon refuses to run a pulled
// config whose bytes don't match the signature (fail-closed).
func TestGhostshipRun_ConfigPull_RejectsTampered(t *testing.T) {
	bucket := "gs-config-pull-bad"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "hello")

	client := e2eS3Client(t)
	good := fmt.Sprintf("id: pull-box\nwriter_id: pull-1\nversion: 1\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src)
	pub := signAndUploadConfig(t, client, bucket, "nas", "pull-1", good)

	// Overwrite the config object with different bytes; the signature no longer matches.
	tampered := fmt.Sprintf("id: evil\nwriter_id: pull-1\nversion: 99\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src)
	putObjectBytes(t, client, bucket, "nas/fleet/pull-1/config.yaml", []byte(tampered))

	out, err := runCargoshipAllowErr(t, "ghostship", "run", "--config-url", "s3://"+bucket+"/nas",
		"--public-key", pub, "--writer-id", "pull-1", "--once", "--region", "us-east-1")
	if err == nil {
		t.Fatalf("daemon should refuse a tampered config, but succeeded:\n%s", out)
	}
	if !strings.Contains(out, "verify config") && !strings.Contains(out, "signature") {
		t.Fatalf("expected a signature-verification failure, got:\n%s", out)
	}
	// Nothing should have been backed up.
	if ids := uploadIDsUnder(t, client, bucket, "writers/pull-1"); len(ids) != 0 {
		t.Fatalf("tampered config must not back up anything, got %d uploads", len(ids))
	}
}

// fleetKeypair generates one config-signing keypair and returns its private/public
// PEM paths, so several config versions can be signed with the SAME key.
func fleetKeypair(t *testing.T) (privPath, pubPath string) {
	t.Helper()
	keyDir := t.TempDir()
	runCargoship(t, "ghostship", "config-keygen", "--out", keyDir)
	return filepath.Join(keyDir, "config-signing-private.pem"), filepath.Join(keyDir, "config-signing-public.pem")
}

// signUploadConfig signs configYAML with privPath and uploads it + its signature to
// <base>/fleet/<writerID>/, overwriting any previous version.
func signUploadConfig(t *testing.T, client *s3.Client, bucket, base, writerID, privPath, configYAML string) {
	t.Helper()
	box := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, box, configYAML)
	runCargoship(t, "ghostship", "sign-config", box, "--key", privPath)
	cp := base + "/fleet/" + writerID
	putObjectFile(t, client, bucket, cp+"/config.yaml", box)
	putObjectFile(t, client, bucket, cp+"/config.yaml.sig", box+".sig")
}

// startPullDaemon starts `ghostship run --config-url` in the background on a 1s interval
// and returns a waitFor(pred) that polls the writer's heartbeat until pred holds (or
// fails the test on timeout). The daemon is stopped via t.Cleanup.
func startPullDaemon(t *testing.T, client *s3.Client, bucket, base, writerID, pubPath string) func(string, func(fleet.WriterStatus) bool) fleet.WriterStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	cmd := exec.CommandContext(ctx, cargoshipBin, "ghostship", "run",
		"--config-url", "s3://"+bucket+"/"+base, "--public-key", pubPath,
		"--writer-id", writerID, "--interval", "1s", "--region", "us-east-1")
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start daemon: %v", err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })

	statusKey := "writers/" + writerID + "/status.json"
	return func(desc string, pred func(fleet.WriterStatus) bool) fleet.WriterStatus {
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			obj, err := client.GetObject(context.Background(), &s3.GetObjectInput{
				Bucket: aws.String(bucket), Key: aws.String(statusKey),
			})
			if err == nil {
				data, _ := io.ReadAll(obj.Body)
				_ = obj.Body.Close()
				var st fleet.WriterStatus
				if json.Unmarshal(data, &st) == nil && pred(st) {
					return st
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for %s\ndaemon output:\n%s", desc, buf.String())
		return fleet.WriterStatus{}
	}
}

// TestGhostshipRun_ConfigPull_RefusesSilentScopeWidening proves #614 slice 3: a validly
// signed config that widens scope (adds a watch path) is refused unless it carries
// allow_scope_expansion — the daemon keeps last-good until the flag is set.
func TestGhostshipRun_ConfigPull_RefusesSilentScopeWidening(t *testing.T) {
	bucket := "gs-config-scope"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src1, src2 := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src1, "a.txt"), "one")
	writeFile(t, filepath.Join(src2, "b.txt"), "two")

	client := e2eS3Client(t)
	priv, pub := fleetKeypair(t)

	v1 := fmt.Sprintf("id: sc\nwriter_id: sc-1\nversion: 1\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src1)
	signUploadConfig(t, client, bucket, "nas", "sc-1", priv, v1)

	waitFor := startPullDaemon(t, client, bucket, "nas", "sc-1", pub)
	waitFor("healthy on v1", func(st fleet.WriterStatus) bool {
		return st.ConfigVersion == 1 && st.ConfigError == "" && st.Healthy()
	})

	// v2 adds a second watch path but does NOT set allow_scope_expansion → refused.
	v2 := fmt.Sprintf("id: sc\nwriter_id: sc-1\nversion: 2\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n  - path: %s\n", bucket, src1, src2)
	signUploadConfig(t, client, bucket, "nas", "sc-1", priv, v2)
	st := waitFor("silent scope-widening refused (still v1)", func(st fleet.WriterStatus) bool {
		return st.ConfigError != "" && st.ConfigVersion == 1
	})
	if !strings.Contains(st.ConfigError, "scope") {
		t.Errorf("config_error should mention scope widening, got %q", st.ConfigError)
	}
	if len(st.Sources) != 1 {
		t.Errorf("still-v1 writer should have 1 source, got %d", len(st.Sources))
	}

	// v3 adds the same path WITH allow_scope_expansion: true → adopted.
	v3 := fmt.Sprintf("id: sc\nwriter_id: sc-1\nversion: 3\nallow_scope_expansion: true\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n  - path: %s\n", bucket, src1, src2)
	signUploadConfig(t, client, bucket, "nas", "sc-1", priv, v3)
	waitFor("scope widening adopted with allow_scope_expansion", func(st fleet.WriterStatus) bool {
		return st.ConfigVersion == 3 && st.ConfigError == "" && len(st.Sources) == 2
	})
}

// TestGhostshipRun_ConfigPull_KeepLastGood proves #614 slice 2b: after a good config
// is running, a later bad (tampered) config on refresh is rejected — the daemon keeps
// running the last-good config, keeps backing up, and surfaces config_error in its
// heartbeat rather than adopting the bad config or stopping.
func TestGhostshipRun_ConfigPull_KeepLastGood(t *testing.T) {
	bucket := "gs-config-keeplast"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "one")

	client := e2eS3Client(t)
	good := fmt.Sprintf("id: kl\nwriter_id: kl-1\nversion: 1\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src)
	pub := signAndUploadConfig(t, client, bucket, "nas", "kl-1", good)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cargoshipBin, "ghostship", "run",
		"--config-url", "s3://"+bucket+"/nas", "--public-key", pub,
		"--writer-id", "kl-1", "--interval", "1s", "--region", "us-east-1")
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()

	tryStatus := func() (fleet.WriterStatus, bool) {
		obj, err := client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String(bucket), Key: aws.String("writers/kl-1/status.json"),
		})
		if err != nil {
			return fleet.WriterStatus{}, false
		}
		defer func() { _ = obj.Body.Close() }()
		data, _ := io.ReadAll(obj.Body)
		var st fleet.WriterStatus
		if json.Unmarshal(data, &st) != nil {
			return fleet.WriterStatus{}, false
		}
		return st, true
	}
	waitFor := func(desc string, pred func(fleet.WriterStatus) bool) fleet.WriterStatus {
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if st, ok := tryStatus(); ok && pred(st) {
				return st
			}
			time.Sleep(200 * time.Millisecond)
		}
		cancel()
		_ = cmd.Wait()
		t.Fatalf("timeout waiting for %s\ndaemon output:\n%s", desc, buf.String())
		return fleet.WriterStatus{}
	}

	// 1) The good config (v1) is running cleanly.
	waitFor("first healthy heartbeat on config v1", func(st fleet.WriterStatus) bool {
		return st.ConfigVersion == 1 && st.ConfigError == "" && st.Healthy()
	})

	// 2) Replace the config object with tampered bytes (the signature no longer matches),
	//    and change a source file so a backup under the last-good config is observable.
	tampered := fmt.Sprintf("id: evil\nwriter_id: kl-1\nversion: 99\ns3_config:\n  bucket: %s\nwatch_paths:\n  - path: %s\n", bucket, src)
	putObjectBytes(t, client, bucket, "nas/fleet/kl-1/config.yaml", []byte(tampered))
	writeFile(t, filepath.Join(src, "a.txt"), "two")

	// 3) The daemon rejects the bad config: it surfaces config_error, keeps running v1
	//    (never adopts version 99), and stays healthy (still backing up last-good).
	st := waitFor("config_error set while still running last-good v1", func(st fleet.WriterStatus) bool {
		return st.ConfigError != "" && st.ConfigVersion == 1
	})
	if !st.Healthy() {
		t.Errorf("daemon should keep backing up under the last-good config: %+v", st.Sources)
	}
}

// TestGhostshipRun_ConfigMultiSource proves config-file mode: each watch_paths entry is
// synced as its own chain under one writer prefix, and archival_rules are ignored with a
// warning.
func TestGhostshipRun_ConfigMultiSource(t *testing.T) {
	bucket := "gs-config"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src1, src2 := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src1, "a.txt"), "alpha")
	writeFile(t, filepath.Join(src2, "b.txt"), "bravo")

	box := filepath.Join(t.TempDir(), "box.yaml")
	writeFile(t, box, fmt.Sprintf(`id: fleet-box
writer_id: fleet-box
version: 42
s3_config:
  bucket: %s
watch_paths:
  - path: %s
  - path: %s
`, bucket, src1, src2))

	runCargoship(t, "ghostship", "run", "--config", box, "--once", "--region", "us-east-1")

	client := e2eS3Client(t)
	if ids := uploadIDsUnder(t, client, bucket, "writers/fleet-box"); len(ids) != 2 {
		t.Fatalf("config multi-source: want 2 uploads under writers/fleet-box/uploads/, got %d: %v", len(ids), ids)
	}

	// #614: the heartbeat reports the config version this writer is running.
	if hb := getStatus(t, client, bucket, "writers/fleet-box/status.json"); hb.ConfigVersion != 42 {
		t.Fatalf("heartbeat config_version = %d, want 42", hb.ConfigVersion)
	}

	// archival_rules are advisory in sync mode: present → run warns but still succeeds.
	boxRules := filepath.Join(t.TempDir(), "box-rules.yaml")
	writeFile(t, boxRules, fmt.Sprintf(`id: rules-box
writer_id: rules-box
s3_config:
  bucket: %s
watch_paths:
  - path: %s
archival_rules:
  - name: legacy
    file_pattern: "*.txt"
`, bucket, src1))
	out, err := runCargoshipAllowErr(t, "ghostship", "run", "--config", boxRules, "--once", "--region", "us-east-1")
	if err != nil {
		t.Fatalf("run with archival_rules should still succeed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "archival_rules") {
		t.Fatalf("expected an archival_rules ignore-warning, got:\n%s", out)
	}
}

// TestGhostshipRun_DisasterRecovery proves the recovery property that anchors the
// fleet trust story (#617): after a ghostship writes a backup, the ENTIRE upload can
// be reconstructed from just the writer URL with `restore --all` — no --writer-id, no
// --config, no local state, i.e. the box that wrote it need not exist. Every file
// comes back byte-identical.
func TestGhostshipRun_DisasterRecovery(t *testing.T) {
	bucket := "gs-dr"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	files := map[string]string{
		"greeting.txt":    "hello disaster recovery",
		"docs/readme.md":  "# readme\nnested",
		"data/report.csv": "a,b,c\n1,2,3\n",
	}
	for name, content := range files {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, content)
	}

	runCargoship(t, "ghostship", "run", src, "s3://"+bucket+"/backups",
		"--writer-id", "lab-nas-1", "--once", "--region", "us-east-1")

	client := e2eS3Client(t)
	ids := uploadIDsUnder(t, client, bucket, "backups/writers/lab-nas-1")
	if len(ids) != 1 {
		t.Fatalf("want 1 upload, got %v", ids)
	}
	uploadURL := "s3://" + bucket + "/backups/writers/lab-nas-1/uploads/" + ids[0]

	// Recover the whole upload to a fresh box — only the URL, no writer identity/state.
	restoreDir := t.TempDir()
	runCargoship(t, "restore", uploadURL, restoreDir, "--all", "--region", "us-east-1")

	for name, want := range files {
		got, err := os.ReadFile(findFileByBase(t, restoreDir, filepath.Base(name)))
		if err != nil {
			t.Fatalf("restored file missing for %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("restored %s = %q, want %q", name, got, want)
		}
	}
}
