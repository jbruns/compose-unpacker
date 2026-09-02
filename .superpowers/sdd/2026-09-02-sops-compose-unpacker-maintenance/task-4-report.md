# Task 4 Report

## Files

- `overlay/commands/sops.go`
- `overlay/commands/sops_test.go`
- `internal/prepare/prepare.go`
- `internal/prepare/prepare_test.go`
- `patches/compose-unpacker.patch`
- validated generated files:
  - `.work/upstream/compose-unpacker/commands/compose_deploy.go`
  - `.work/upstream/compose-unpacker/commands/swarm_deploy.go`

## TDD Record

### Helper RED

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /bin/sh -lc '/usr/local/go/bin/go run ./cmd/prepare && cd /repo/.work/upstream/compose-unpacker && /usr/local/go/bin/go test ./commands -count=1'
# github.com/portainer/compose-unpacker/commands [github.com/portainer/compose-unpacker/commands.test]
commands/sops_test.go:29:2: undefined: decryptSOPS
commands/sops_test.go:37:3: undefined: decryptSOPS
commands/sops_test.go:40:9: undefined: decryptSOPSFiles
FAIL	github.com/portainer/compose-unpacker/commands [build failed]
FAIL
```

### Helper GREEN

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /bin/sh -lc '/usr/local/go/bin/gofmt -w overlay/commands && /usr/local/go/bin/go run ./cmd/prepare && cd /repo/.work/upstream/compose-unpacker && /usr/local/go/bin/go test ./commands -count=1'
ok  	github.com/portainer/compose-unpacker/commands	0.171s
```

### Source-order RED

```console
$ python3 - <<'PY'
from pathlib import Path
for name in ('compose_deploy.go', 'swarm_deploy.go'):
    source = Path('.work/upstream/compose-unpacker/commands', name).read_text()
    assert source.index('decryptSOPSFiles(') < source.index('deployer.Deploy(')
PY
Traceback (most recent call last):
  File "<stdin>", line 4, in <module>
ValueError: substring not found
```

### Prepare allowlist RED

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /usr/local/go/bin/go test ./internal/prepare -run TestRunPreparesImmutableUpstreamTrees -count=1
--- FAIL: TestRunPreparesImmutableUpstreamTrees (0.01s)
    prepare_test.go:70: Run() error = verify compose-unpacker changes: dirty paths = [commands/compose_deploy.go commands/sops.go commands/sops_test.go commands/swarm_deploy.go go.mod sopsdecrypt/discover_test.go], want [commands/compose_deploy.go commands/sops.go commands/sops_test.go go.mod sopsdecrypt/discover_test.go]
FAIL
FAIL	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.040s
FAIL
```

### Prepare allowlist GREEN

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /usr/local/go/bin/go test ./internal/prepare -run TestRunPreparesImmutableUpstreamTrees -count=1
ok  	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.042s
```

### Zero-context patch support RED

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /usr/local/go/bin/go test ./internal/prepare -run TestRunPreparesImmutableUpstreamTrees -count=1
--- FAIL: TestRunPreparesImmutableUpstreamTrees (0.01s)
    prepare_test.go:94: run commands = []string{"git init", "git remote add origin https://github.com/portainer/compose-unpacker.git", "git fetch --depth=1 origin refs/tags/2.45.0", "git checkout --detach 23c8e42176c521cb6745b3ea95233d3a68bbe031", "git init", "git remote add origin https://github.com/portainer/portainer.git", "git fetch --depth=1 origin refs/tags/2.45.0", "git checkout --detach d79ba726cd54395a54cca5e9180609ce52fa7a4f", "go mod edit -replace=github.com/portainer/portainer=../portainer", "git apply --check /repo/.tmp/test-temp/TestRunPreparesImmutableUpstreamTrees1100993172/001/patches/compose-unpacker.patch", "git apply /repo/.tmp/test-temp/TestRunPreparesImmutableUpstreamTrees1100993172/001/patches/compose-unpacker.patch"}, want []string{"git init", "git remote add origin https://github.com/portainer/compose-unpacker.git", "git fetch --depth=1 origin refs/tags/2.45.0", "git checkout --detach 23c8e42176c521cb6745b3ea95233d3a68bbe031", "git init", "git remote add origin https://github.com/portainer/portainer.git", "git fetch --depth=1 origin refs/tags/2.45.0", "git checkout --detach d79ba726cd54395a54cca5e9180609ce52fa7a4f", "go mod edit -replace=github.com/portainer/portainer=../portainer", "git apply --check --unidiff-zero /repo/.tmp/test-temp/TestRunPreparesImmutableUpstreamTrees1100993172/001/patches/compose-unpacker.patch", "git apply --unidiff-zero /repo/.tmp/test-temp/TestRunPreparesImmutableUpstreamTrees1100993172/001/patches/compose-unpacker.patch"}
FAIL
FAIL	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.042s
FAIL
```

### Zero-context patch support GREEN

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /usr/local/go/bin/go test ./internal/prepare -run TestRunPreparesImmutableUpstreamTrees -count=1
ok  	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.042s
```

### Full prepare package RED after wiring `--unidiff-zero`

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /bin/sh -lc '/usr/local/go/bin/go test ./cmd/prepare ./internal/prepare -count=1 && /usr/local/go/bin/go vet ./cmd/prepare ./internal/prepare && cd /repo/.work/upstream/compose-unpacker && /usr/local/go/bin/go test ./... -count=1 && /usr/local/go/bin/go vet ./...'
?   	github.com/jbruns/compose-unpacker-sops/cmd/prepare	[no test files]
--- FAIL: TestRunReportsFailingStageAndStderr (0.02s)
    prepare_test.go:185: Run() error = "verify compose-unpacker changes: unexpected output command \"git status --porcelain --untracked-files=all\" in \"/repo/.tmp/test-temp/TestRunReportsFailingStageAndStderr4209324010/001/.work/upstream/compose-unpacker\"", want apply patch stage
FAIL
FAIL	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.053s
FAIL
```

### Final GREEN

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /bin/sh -lc '/usr/local/go/bin/gofmt -w overlay/commands internal/prepare && /usr/local/go/bin/go test ./cmd/prepare ./internal/prepare -count=1 && /usr/local/go/bin/go vet ./cmd/prepare ./internal/prepare && cd /repo/.work/upstream/compose-unpacker && /usr/local/go/bin/go test ./... -count=1 && /usr/local/go/bin/go vet ./...'
?   	github.com/jbruns/compose-unpacker-sops/cmd/prepare	[no test files]
ok  	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.059s
?   	github.com/portainer/compose-unpacker	[no test files]
?   	github.com/portainer/compose-unpacker/auth	[no test files]
ok  	github.com/portainer/compose-unpacker/commands	0.213s
?   	github.com/portainer/compose-unpacker/exec	[no test files]
?   	github.com/portainer/compose-unpacker/log	[no test files]
ok  	github.com/portainer/compose-unpacker/sopsdecrypt	0.138s

$ test "$(grep -R 'Strs(\"env\"' .work/upstream/compose-unpacker/commands/compose_deploy.go | wc -l | tr -d ' ')" = "0"
$ python3 -c "from pathlib import Path
for name in ('compose_deploy.go', 'swarm_deploy.go'):
    source = Path('.work/upstream/compose-unpacker/commands', name).read_text()
    assert source.index('decryptSOPSFiles(') < source.index('deployer.Deploy(')"
$ git --no-pager diff --check
```

## Self-review

- `overlay/commands/sops.go` is a minimal helper: it keeps `decryptSOPS` injectable for focused tests, passes the resolved compose paths and env through unchanged, uses `sopsdecrypt.DefaultBinaryPath`, and wraps failures with `exec.ErrDeployComposeFailure`.
- `patches/compose-unpacker.patch` is the minimal persistent patch requested: only `commands/compose_deploy.go` and `commands/swarm_deploy.go` change, with Compose still logging `envCount` instead of raw environment values.
- `internal/prepare` was updated only where this task required it: the generated-tree allowlist now includes `commands/swarm_deploy.go`, and patch application uses `--unidiff-zero` so the minimized patch applies cleanly.
- Verified in the regenerated upstream tree that both deployment paths call `decryptSOPSFiles` after `composeFilePaths` resolution and before `deployer.Deploy`.

## Concerns

- Non-blocking: repo-root `go test ./...` is no longer a meaningful validator for `overlay/commands`, because that overlay package intentionally imports compose-unpacker module paths and is validated in the regenerated upstream tree instead.

## Fix Round 1

### Changes

- Removed `--unidiff-zero` from `internal/prepare/prepare.go` so patch verification/application now relies on ordinary contextual hunks and fails on semantic drift.
- Updated the prepare fake-runner assertions in `internal/prepare/prepare_test.go` to expect plain `git apply --check` / `git apply`.
- Added `TestComposeUnpackerPatchAppliesToExpectedSourceAndRejectsDrift`, which initializes real Git fixture repositories from `internal/prepare/testdata/`, proves the persistent patch applies to the expected upstream sources, and proves `git apply --check` rejects drifted sources before apply.
- Regenerated `patches/compose-unpacker.patch` from the prepared compose-unpacker tree with ordinary contextual hunks around both deploy-file edits.

### Covering tests and verification

#### RED: drifted-source rejection test against zero-context apply

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /usr/local/go/bin/go test ./internal/prepare -run TestComposeUnpackerPatchAppliesToExpectedSourceAndRejectsDrift -count=1
--- FAIL: TestComposeUnpackerPatchAppliesToExpectedSourceAndRejectsDrift (0.17s)
    prepare_test.go:221: applyPatch() error = nil, want git apply --check drift rejection
FAIL
FAIL	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.198s
FAIL
```

#### Regenerate contextual persistent patch

```console
$ git -C .work/upstream/compose-unpacker diff -- commands/compose_deploy.go commands/swarm_deploy.go > patches/compose-unpacker.patch && test -s patches/compose-unpacker.patch && /usr/bin/sed -n '1,80p' patches/compose-unpacker.patch
diff --git a/commands/compose_deploy.go b/commands/compose_deploy.go
index 5abfa74..d9e8db6 100644
--- a/commands/compose_deploy.go
+++ b/commands/compose_deploy.go
@@ -42,7 +42,7 @@ func (cmd *DeployCommand) Run(cmdCtx *exec.CommandExecutionContext) error {
		Str("repository", cmd.GitRepository).
		Strs("composePath", cmd.ComposeRelativeFilePaths).
		Str("destination", cmd.Destination).
-		Strs("env", cmd.Env).
+		Int("envCount", len(cmd.Env)).
		Bool("skipTLSVerify", cmd.SkipTLSVerify).
		Msg("Deploying Compose stack from Git repository")

@@ -123,6 +123,11 @@ func (cmd *DeployCommand) Run(cmdCtx *exec.CommandExecutionContext) error {
		composeFilePaths[i] = filesystem.JoinPaths(clonePath, cmd.ComposeRelativeFilePaths[i])
	}

+	if err := decryptSOPSFiles(cmdCtx.Context, composeFilePaths, cmd.Env); err != nil {
+		log.Error().Err(err).Msg("Failed to decrypt SOPS files")
+		return err
+	}
+
	log.Info().
		Strs("composeFilePaths", composeFilePaths).
		Str("workingDirectory", clonePath).
diff --git a/commands/swarm_deploy.go b/commands/swarm_deploy.go
index 8f5fe52..b80e262 100644
--- a/commands/swarm_deploy.go
+++ b/commands/swarm_deploy.go
@@ -124,6 +124,11 @@ func (cmd *SwarmDeployCommand) Run(cmdCtx *exec.CommandExecutionContext) error {
		composeFilePaths[i] = filesystem.JoinPaths(clonePath, cmd.ComposeRelativeFilePaths[i])
	}

+	if err := decryptSOPSFiles(cmdCtx.Context, composeFilePaths, cmd.Env); err != nil {
+		log.Error().Err(err).Msg("Failed to decrypt SOPS files")
+		return err
+	}
+
	registries := exec.ParseRegistryCredentials(cmd.Registry)

	log.Info().
```

#### GREEN: focused prepare test, vet, regenerated-tree tests/vet, redaction, order, no-unidiff, diff-check

```console
$ docker run --rm --platform linux/amd64 --user "$(id -u):$(id -g)" -e HOME=/repo/.tmp/home -e TMPDIR=/repo/.tmp/test-temp -e GOCACHE=/repo/.tmp/go-build -e GOMODCACHE=/repo/.tmp/go-mod -e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0='*' -v "$PWD:/repo" -w /repo golang:1.26.6 /bin/sh -lc '/usr/local/go/bin/go test ./internal/prepare -run "TestRunPreparesImmutableUpstreamTrees|TestRunReportsFailingStageAndStderr|TestComposeUnpackerPatchAppliesToExpectedSourceAndRejectsDrift" -count=1 && /usr/local/go/bin/go vet ./internal/prepare && /usr/local/go/bin/go run ./cmd/prepare && cd /repo/.work/upstream/compose-unpacker && /usr/local/go/bin/go test ./... -count=1 && /usr/local/go/bin/go vet ./...'
ok  	github.com/jbruns/compose-unpacker-sops/internal/prepare	0.195s
?   	github.com/portainer/compose-unpacker	[no test files]
?   	github.com/portainer/compose-unpacker/auth	[no test files]
ok  	github.com/portainer/compose-unpacker/commands	0.209s
?   	github.com/portainer/compose-unpacker/exec	[no test files]
?   	github.com/portainer/compose-unpacker/log	[no test files]
ok  	github.com/portainer/compose-unpacker/sopsdecrypt	0.145s

$ test "$(grep -R 'Strs(\"env\"' .work/upstream/compose-unpacker/commands/compose_deploy.go | wc -l | tr -d ' ')" = "0" && python3 - <<'PY'
from pathlib import Path
for name in ("compose_deploy.go", "swarm_deploy.go"):
    source = Path(".work/upstream/compose-unpacker/commands", name).read_text()
    assert source.index("decryptSOPSFiles(") < source.index("deployer.Deploy(")
PY
! grep -R -- '--unidiff-zero' internal/prepare patches/compose-unpacker.patch && git -C .work/upstream/compose-unpacker --no-pager diff --check
# no output (exit 0)
```
