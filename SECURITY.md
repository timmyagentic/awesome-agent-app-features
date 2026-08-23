# Security policy

Please report vulnerabilities privately through GitHub's **Report a
vulnerability** flow when available. Do not send secrets or exploit details to
the public feedback relay.

This repository is a feature foundation. A supported security report should
identify a flaw in the reusable core, relay template, documented invariant, or
example that could cause unauthorized feedback submission, data disclosure,
unsafe release selection, integrity bypass, executable corruption, or rollback
failure.

No public version has been released yet. After the first v1 release, support
will follow the latest `v1.x` GitHub Release. Security fixes will use new
immutable patch versions; release tags will never be moved.

See [docs/security.md](docs/security.md) for the threat model and residual risks.
