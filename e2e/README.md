# E2E Tests

Runs Firecracker VMs via Nomad and checks basic lifecycle.

## Requirements

- Linux with KVM (`/dev/kvm`)
- `firecracker` and `jailer` binaries in `$PATH`
- `nomad` binary in `$PATH`
- Root privileges (jailer requirement)


## Usage

Start the Nomad dev agent with the plugin (downloads images on first run):

```sh
make hack
```

In a second terminal, run the tests:

```sh
make e2e
```
