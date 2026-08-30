# Release process

Releases are immutable Go module tags plus reproducible source archives attached to a GitHub Release.

1. Update `CHANGELOG.md` and verify the intended version follows SemVer. For first publication, change the remote entry and each feature manifest to `release_status: released` and set `since` to that exact first tag; later releases preserve the original `since` value.
2. Run `make verify` from a clean worktree. This includes independent Relay subtree extraction, generated Worker binding types, and the stateless lock validator contracts.
3. Confirm `api/v1.txt`, shared protocol fixtures, the remote entry, feature manifests, lock schema/validator, and compatibility policy changed only by deliberate review.
4. Merge the release commit to `main` with all required checks green.
5. Stop until version publication is explicitly authorized. Creating or pushing a tag is not part of ordinary stabilization work.
6. After authorization, create an annotated tag on the exact current `main` commit and push it: `git tag -a vX.Y.Z -m "vX.Y.Z"`.
7. Manually dispatch `Publish release (manual)` with that exact tag and the required `publish <tag>` confirmation. It validates the tag, manifests, and changelog, reruns all gates, builds a deterministic `git archive` with `gzip -n`, attests the archive, publishes its SHA-256, and creates the GitHub Release. Tag pushes alone do not trigger it.
8. In a directory outside this repository with `GOWORK=off`, resolve `github.com/timmyagentic/awesome-agent-app-features@vX.Y.Z`, compile a consumer without `replace`, independently extract and verify the Relay subtree, and validate a real host lock against the tagged source.

Never move or reuse a published tag. If a release workflow fails after the tag is public, fix forward with the next patch version unless the tag has not propagated anywhere and repository policy explicitly permits deletion.

The release workflow does not deploy the Cloudflare relay or modify any host product. Those are separately authorized operator actions.
