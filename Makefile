.PHONY: verify go-verify relay-verify source-subtree-verify script-verify fuzz

verify: go-verify relay-verify source-subtree-verify script-verify

go-verify:
	test -z "$$(gofmt -l .)"
	go mod tidy -diff
	go test -race ./...
	go vet ./...
	GOOS=windows GOARCH=amd64 go build ./...

relay-verify:
	npm ci --prefix relay/cloudflare --ignore-scripts
	node --test internal/contract/relay_contract.test.mjs
	npm test --prefix relay/cloudflare
	npm run check --prefix relay/cloudflare
	npm run typecheck --prefix relay/cloudflare
	npm run types:check --prefix relay/cloudflare
	npm run validate:worker --prefix relay/cloudflare
	npm audit --prefix relay/cloudflare --audit-level=high

source-subtree-verify:
	sh scripts/verify-relay-subtree.sh

script-verify:
	sh -n scripts/build-source-release.sh
	sh -n scripts/verify-relay-subtree.sh

fuzz:
	go test ./feedback -run '^$$' -fuzz FuzzBuilderProducesBoundedValidApprovedJSON -fuzztime 10s
	go test ./feedback -run '^$$' -fuzz FuzzRedactAlwaysReturnsValidUTF8 -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzChecksumManifestParser -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzArchiveEntryMatchNeverAcceptsTraversal -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzStableVersionComparison -fuzztime 10s
	go test ./internal/lockcheck -run '^$$' -fuzz FuzzJoinedPathNeverEscapesRoot -fuzztime 10s
