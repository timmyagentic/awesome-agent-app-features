# Updater integration contract

The updater is a transaction core, not an update UI. Every chat, CLI, or GUI
adapter must delegate to one configured `updater.Updater`.

```text
latest stable release
  -> exact archive + exact checksums asset on that release
  -> bounded download
  -> SHA-256
  -> extract one configured binary in target directory
  -> staged version probe
  -> backup + same-filesystem rename
  -> installed version probe
  -> remove backup, or rollback on failure
```

The built-in GitHub source uses `/releases/latest` and independently validates
the response. It does not infer assets from another tag or download a checksum
from a moving branch.

The checksum protects archive integrity only. Add a separately rooted signature
or provenance verifier in a higher-assurance release pipeline.

The MVP intentionally does not generalize package-manager installs. An npm,
Homebrew, or other adapter must preserve the same stable target and post-install
version contract and define its own honest rollback behavior. The standalone
updater refuses a symlink executable path so it does not silently rewrite a
package-manager target.
