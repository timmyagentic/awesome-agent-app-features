.PHONY: verify go-verify relay-verify source-subtree-verify author-verify script-verify new-feature fuzz

verify: go-verify relay-verify source-subtree-verify author-verify script-verify

go-verify:
	test -z "$$(gofmt -l .)"
	go mod tidy -diff
	go test -race ./...
	go vet ./...
	GOOS=windows GOARCH=amd64 go build ./...

relay-verify:
	sh relay/cloudflare/verify.sh
	node --test internal/contract/relay_contract.test.mjs

source-subtree-verify:
	sh scripts/verify-relay-subtree.sh

author-verify:
	go run ./cmd/feature-author validate --root .

new-feature:
	test -n "$(ID)"
	test -n "$(NAME)"
	go run ./cmd/feature-author new --root . --id "$(ID)" --name "$(NAME)" --kind "$(or $(KIND),go)" $(if $(RUNTIME),--runtime "$(RUNTIME)",)

script-verify:
	sh -n scripts/build-source-release.sh
	sh -n scripts/verify-relay-subtree.sh
	sh -n scripts/verify-remote-consumer.sh
	sh -n relay/cloudflare/verify.sh

fuzz:
	go test ./feedback -run '^$$' -fuzz FuzzBuilderProducesBoundedValidApprovedJSON -fuzztime 10s
	go test ./feedback -run '^$$' -fuzz FuzzRedactAlwaysReturnsValidUTF8 -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzChecksumManifestParser -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzArchiveEntryMatchNeverAcceptsTraversal -fuzztime 10s
	go test ./updater -run '^$$' -fuzz FuzzStableVersionComparison -fuzztime 10s
	go test ./internal/lockcheck -run '^$$' -fuzz FuzzJoinedPathNeverEscapesRoot -fuzztime 10s
