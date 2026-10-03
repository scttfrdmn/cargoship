package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// credentialsPreflight reports a clear error when an AWS credentials file exists at the
// path the SDK will consult but this process cannot read it (#692).
//
// Without this the failure surfaces as the SDK's LAST resort giving up:
//
//	failed to refresh cached credentials, no EC2 IMDS role found,
//	operation error ec2imds: GetMetadata, request canceled, context deadline exceeded
//
// which names IMDS and says nothing about an unreadable file. That is how it actually
// presented on a real NAS: `ghostship init` writes aws-credentials 0600 owned by the
// operator, the emitted compose mounts it into a container running as uid 65532, and the
// agent crash-looped with an IMDS timeout.
//
// A MISSING file is not an error — environment variables, an instance role or a mounted
// web-identity token are all legitimate. Only "present but unreadable" is reported, since
// that configuration is always a mistake.
func credentialsPreflight() error {
	path := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil // can't locate it; let the SDK decide
		}
		path = filepath.Join(home, ".aws", "credentials")
	}

	// #nosec G304,G703 -- path is the AWS SDK's OWN credentials location: either
	// $AWS_SHARED_CREDENTIALS_FILE, which the SDK itself is about to read, or
	// $HOME/.aws/credentials. gosec flags it as tainted because it comes from the
	// environment, but we only ask whether it is readable and never use its contents,
	// so inspecting a file the SDK will open anyway adds no exposure.
	fi, err := os.Stat(path)
	if err != nil {
		return nil // absent, or unstattable: not our call to make
	}
	if fi.IsDir() {
		return fmt.Errorf("aws credentials path %s is a directory, not a file "+
			"(a container bind-mount creates a directory when the host path does not exist)", path)
	}

	f, err := os.Open(path) // #nosec G304,G703 -- same SDK-owned path as the Stat above
	if err == nil {
		_ = f.Close()
		return nil
	}
	if !errors.Is(err, fs.ErrPermission) {
		return nil // some other problem; let the SDK produce its own error
	}

	detail := fmt.Sprintf("mode %04o", fi.Mode().Perm())
	if uid, ok := fileOwnerUID(fi); ok {
		detail = fmt.Sprintf("mode %04o owned by uid %d", fi.Mode().Perm(), uid)
	}
	return fmt.Errorf("aws credentials file %s exists but is not readable by uid %d (%s). "+
		"A bundle's aws-credentials is 0600 owned by the operator who ran 'ghostship init', so a "+
		"container running as a different user cannot read it: run 'chown %d:%d aws-credentials' on "+
		"the host and keep 0600. Left unfixed, the AWS SDK falls through to EC2 IMDS and fails with "+
		"an error that does not mention this file (see issue #692)",
		path, os.Getuid(), detail, os.Getuid(), os.Getgid())
}
