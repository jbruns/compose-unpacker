# Unpacker

Build:

```
# To build the first argument, just our binary nothing else . Convenience for dev.
make

# Same as make (currently downloads binaries even if you don't need to which is not useful)
make build

# To make and build everything, this needs to ensure everything is there before creating the image so must depend on everything
make image
```

Run:

```
docker run --rm -v /tmp/unpacker:/tmp/unpacker -v /var/run/docker.sock:/var/run/docker.sock portainer/compose-unpacker deploy https://github.com/deviantony/docker-workbench.git mystack /tmp/unpacker compose/relative-paths/web-static-content/docker-compose.yml 
```

**IMPORTANT NOTE**: the bind mount on the host **MUST MATCH** the bind mount inside the container for any relative asset to be loaded properly. `-v /tmp/unpacker:/tmp/unpacker` will work fine but `-v /tmp/unpacker-test:/tmp/unpacker` WILL NOT.

## Version updates

Use `go run ./cmd/update-versions -manifest versions.json -check` to check
Portainer LTS and SOPS releases, or replace `-check` with `-write` to atomically
update the manifest. Set `GITHUB_TOKEN` optionally for authenticated GitHub API
requests.

Exit codes are `0` for success/current, `2` when `-check` finds an update, and
`1` for invalid arguments, API failures, invalid upstream data, or write
failures.
