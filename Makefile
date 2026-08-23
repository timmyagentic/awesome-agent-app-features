.PHONY: verify go-verify relay-verify script-verify fuzz

verify: go-verify relay-verify script-verify

go-verify:
	test -z "$$(gofmt -l .)"
	go mod tidy -diff
	go test -race ./...
	go vet ./...
	GOOS=windows GOARCH=amd64 go build ./...

relay-verify:
	npm ci --prefix relay/cloudflare --ignore-scripts
	npm test --prefix relay/cloudflare
	npm run check --prefix relay/cloudflare
	npm run validate:worker --prefix relay/cloudflare
	npm audit --prefix relay/cloudflare --audit-level=high

script-verify:
	sh -n scripts/build-source-release.sh

fuzz:
	go test ./feedback -run '^$$' -fuzz FuzzBuilderProducesBoundedValidApprovedJSON -fuzztime 10s
	go test ./feedback -run '^$$' -fuzz FuzzRedactAlwaysReturnsValidUTF8 -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzChecksumManifestParser -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzArchiveEntryMatchNeverAcceptsTraversal -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzStableVersionComparison -fuzztime 10s
