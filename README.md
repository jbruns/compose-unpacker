# compose-unpacker with SOPS

This repository builds Portainer's `compose-unpacker` with a small, reviewed
overlay that decrypts SOPS files before Compose or Swarm deployment. The image
keeps the vanilla Portainer runtime, entrypoint, and deployment behavior; unlike
the vanilla image, it includes a checksum-verified SOPS binary and automatically
decrypts stack assets.

Only **Linux amd64** is supported. `versions.json` is authoritative for all
inputs. The current manifest pins Portainer **2.45.0**, SOPS **v3.13.3**, and
overlay revision **3**.

## Images

Releases use `ghcr.io/jbruns/compose-unpacker` and publish three tags:

| Tag | Meaning |
| --- | --- |
| `2.45.0-sops.3` | Immutable Portainer, SOPS overlay revision |
| `2.45.0-sops` | Moves to the latest overlay for Portainer 2.45.0 |
| `lts-sops` | Moves to the currently maintained Portainer LTS image |

Use the immutable tag in production:

```text
ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3
```

Moving tags are convenient for evaluation but can resolve to a different image
after a maintenance release.

## Configure Portainer

Configure Portainer to use the image, then add `SOPS_AGE_KEY` to the stack
environment in Portainer. Its value is the complete age secret key accepted by
SOPS. Treat it as a secret: do not put it in the Compose file, repository, image,
or logs.

For every resolved Compose file, the unpacker recursively scans its containing
directory. It skips `.git`, does not follow symlinks, and decrypts regular files
whose names contain `.sops.`. The first marker is removed:

```text
config.sops.env       -> config.env
nested/app.sops.yaml  -> nested/app.yaml
```

Decryption occurs before Compose parses files or deployment begins. A discovery,
SOPS, write, synchronization, or rename error aborts the deployment; there is no
plaintext or stale-output fallback. Restrictive temporary files are atomically
renamed to their destinations and removed on failure. Successful plaintext
outputs remain in Portainer's cloned stack directory so `env_file`, configs, and
bind mounts continue to work. They are removed or retained with that stack
directory according to Portainer's normal lifecycle.

## Local development

Prerequisites:

- Go at the version in `versions.json`/`go.mod`;
- Git;
- Docker with Buildx and Linux amd64 support; and
- network access to GitHub, GHCR, Docker Hub, and the Go module proxy.

The Make targets run Go validation in the manifest-pinned Go container, so a
host Go installation is needed only for direct `go run` commands. Run:

```sh
make validate
make image IMAGE=ghcr.io/jbruns/compose-unpacker:test
make test-image IMAGE=ghcr.io/jbruns/compose-unpacker:test
```

`make validate` runs owned and prepared-upstream tests and vet, formatting and
shell checks, the real SOPS/age integration test, pinned golangci-lint, and, when
`IMAGE` is supplied, the image build and smoke/layer checks:

```sh
make validate IMAGE=ghcr.io/jbruns/compose-unpacker:test
```

## Update and release maintenance

The update workflow runs weekly and can be started manually. It resolves the
latest Portainer LTS and stable SOPS release, updates immutable commits, digest,
asset URL, and checksum in `versions.json`, performs full validation, and opens
or refreshes an update pull request. No update run publishes an image.

Publication is a separate gate. Only a relevant change merged to `main`, or an
explicit manual release dispatch on `main`, runs full prepublication validation
and publishes. The workflow authenticates to GHCR with `GITHUB_TOKEN`, refuses
to overwrite an existing immutable tag, then updates the two moving aliases.

Before enabling either scheduled updates or releases for this repository:

1. create or select `main`, merge the maintained implementation into it, and set
   `main` as the GitHub default branch;
2. allow the workflows their declared repository permissions; and
3. publish one release, then make the `compose-unpacker` GHCR package public if
   unauthenticated Portainer nodes must pull it. New GHCR packages may otherwise
   remain private and require registry credentials.

### Overlay revision

`overlayRevision` is part of the immutable tag. Set it to `1` when Portainer
changes to a new version. For the same Portainer version, increment it whenever
a release-impacting overlay, patch, build input, manifest helper, or runtime
build path changes. SOPS-only manifest updates also increment it. Never reuse or
overwrite an immutable tag.

Validation enforces the revision rule for the release-impacting paths. Run it
against the branch base in CI by setting `BASE_REF`.

### Repair the integration patch

An upstream Portainer change can make `patches/compose-unpacker.patch` stop
applying. Repair it against the exact commits selected in `versions.json`:

1. run `make clean`, then `make prepare` and inspect the reported failed hunk;
2. in `.work/upstream/compose-unpacker`, restore the affected tracked files to
   the checked-out upstream commit and reapply only the required decryption call
   and environment-log redaction;
3. regenerate `patches/compose-unpacker.patch` with
   `git -C .work/upstream/compose-unpacker diff -- commands/compose_deploy.go commands/swarm_deploy.go > patches/compose-unpacker.patch`;
4. review the patch for unrelated changes, increment/reset `overlayRevision`,
   and rerun `make clean` followed by full `make validate IMAGE=...`.

Owned new files belong under `overlay`; the patch should contain only minimal
edits to existing upstream files.

### Verify release evidence

After installing the GitHub CLI, verify GitHub's build-provenance attestation:

```sh
gh attestation verify \
  oci://ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
  --repo jbruns/compose-unpacker
```

BuildKit also attaches an SPDX SBOM and provenance to the pushed digest. Inspect
them with a recent Buildx:

```sh
docker buildx imagetools inspect \
  ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
  --format '{{ json .SBOM }}'
docker buildx imagetools inspect \
  ghcr.io/jbruns/compose-unpacker:2.45.0-sops.3 \
  --format '{{ json .Provenance }}'
```

The old manual notes are historical context, not current instructions. Do not
use their release branches, branch-wide import replacement, pseudo-version
lookup, Docker Hub login, or manual `build_and_push.sh` process.
