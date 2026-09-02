package sopsdecrypt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SOPS_TEST_HELPER") == "1" {
		runSOPSHelper()
		return
	}
	os.Exit(m.Run())
}

func TestBinaryRunnerUsesAbsoluteSourceAndExactEnvironment(t *testing.T) {
	t.Parallel()

	env := []string{
		"SOPS_TEST_HELPER=1",
		"SOPS_TEST_ACTION=environment",
		"DEPLOYMENT=value",
	}
	var destination bytes.Buffer
	err := (binaryRunner{binary: os.Args[0]}).Decrypt(
		context.Background(),
		"relative.sops.env",
		&destination,
		env,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join(env, "\n") + "\n"
	if destination.String() != want {
		t.Fatalf("environment = %q, want %q", destination.String(), want)
	}
}

func TestBinaryRunnerRedactsStderrAndEnvironmentValues(t *testing.T) {
	t.Parallel()

	source := filepath.Join(t.TempDir(), "app.sops.env")
	env := []string{
		"SOPS_TEST_HELPER=1",
		"SOPS_TEST_ACTION=fail",
		"SOPS_AGE_KEY=secret-test-key",
	}
	err := (binaryRunner{binary: os.Args[0]}).Decrypt(context.Background(), source, &bytes.Buffer{}, env)
	if err == nil {
		t.Fatal("Decrypt() error = nil")
	}
	want := fmt.Sprintf("decrypt %s: sops exited with status 23", source)
	if err.Error() != want {
		t.Fatalf("Decrypt() error = %q, want %q", err, want)
	}
	for _, sensitive := range []string{"secret-test-key", "provider stderr"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatalf("Decrypt() error %q contains sensitive stderr", err)
		}
	}
}

func TestBinaryRunnerReturnsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	env := []string{
		"SOPS_TEST_HELPER=1",
		"SOPS_TEST_ACTION=block",
	}
	err := (binaryRunner{binary: os.Args[0]}).Decrypt(ctx, "app.sops.env", &bytes.Buffer{}, env)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Decrypt() error = %v, want context deadline exceeded", err)
	}
}

func runSOPSHelper() {
	if len(os.Args) != 3 || os.Args[1] != "decrypt" || !filepath.IsAbs(os.Args[2]) {
		fmt.Fprintln(os.Stderr, "invalid helper arguments")
		os.Exit(42)
	}

	switch os.Getenv("SOPS_TEST_ACTION") {
	case "environment":
		for _, value := range os.Environ() {
			fmt.Println(value)
		}
	case "plaintext":
		fmt.Print("VALUE=decrypted\n")
	case "fail":
		fmt.Fprintln(os.Stderr, "provider stderr: secret-test-key")
		os.Exit(23)
	case "block":
		select {}
	default:
		fmt.Fprintln(os.Stderr, "invalid helper action")
		os.Exit(43)
	}
}
