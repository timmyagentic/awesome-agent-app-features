#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
  printf 'usage: %s <module> <commit> <source-root> <consumer-root>\n' "$0" >&2
  exit 2
fi

remote_module=$1
remote_commit=$2
source_root=$3
consumer_root=$4

test "${#remote_commit}" -eq 40
test -f "$source_root/features/index.json"
mkdir -p "$consumer_root/infrastructure"

cd "$consumer_root"
if [ ! -f go.mod ]; then
  go mod init example.com/remote-agent-consumer
fi
GOWORK=off GOPROXY=direct go get "$remote_module@$remote_commit"
GOWORK=off GOPROXY=direct go test "$remote_module/compat/v1"
GOWORK=off GOPROXY=direct go run "$remote_module/cmd/feature-author@$remote_commit" validate --root "$source_root"
module_version=$(GOWORK=off go list -m -f '{{.Version}}' "$remote_module")
verified_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
lock_path="$consumer_root/agent-app-features.lock.json"
jq -n --arg commit "$remote_commit" '{
  schema: 1,
  source: {repository: "timmyagentic/awesome-agent-app-features", commit: $commit},
  features: []
}' > "$lock_path"

jq -r '.features[].manifest' "$source_root/features/index.json" | while IFS= read -r manifest_relative; do
  manifest_path="$source_root/$manifest_relative"
  test -f "$manifest_path"
  release_status=$(jq -r '.release_status' "$manifest_path")
  [ "$release_status" = released ] || continue

  feature_id=$(jq -r '.id' "$manifest_path")
  contract=$(jq -r '.contract' "$manifest_path")
  deliveries_file="$consumer_root/.deliveries-$feature_id.json"
  files_file="$consumer_root/.files-$feature_id.json"
  jq -n '[]' > "$deliveries_file"
  jq -n '["go.mod", "go.sum"]' > "$files_file"
  checks='["remote generic consumer"]'

  jq -c '.delivery[]' "$manifest_path" | while IFS= read -r delivery; do
    mode=$(printf '%s' "$delivery" | jq -r '.mode')
    case "$mode" in
      go-module)
        printf '%s' "$delivery" | jq -r '.packages[]' | while IFS= read -r package_path; do
          GOWORK=off GOPROXY=direct go test "$package_path"
          temporary_json="$deliveries_file.tmp"
          jq \
            --arg source "$package_path" \
            --arg version "$module_version" \
            '. + [{mode:"go-module", source:$source, target:"go.mod", version:$version}]' \
            "$deliveries_file" > "$temporary_json"
          mv "$temporary_json" "$deliveries_file"
        done
        ;;
      source-subtree)
        source_relative=$(printf '%s' "$delivery" | jq -r '.path')
        source_directory="$source_root/$source_relative"
        target_relative="infrastructure/$feature_id/$(basename "$source_relative")"
        target_directory="$consumer_root/$target_relative"
        mkdir -p "$(dirname "$target_directory")"
        cp -R "$source_directory" "$target_directory"
        verify_relative=$(printf '%s' "$delivery" | jq -r '.verify')
        test -n "$verify_relative"
        sh "$target_directory/$verify_relative"
        witness=$(find "$target_directory" -type f -print | LC_ALL=C sort | sed -n '1p')
        test -n "$witness"
        witness_relative=${witness#"$consumer_root/"}
        temporary_json="$deliveries_file.tmp"
        jq \
          --arg source "$source_relative" \
          --arg target "$target_relative" \
          '. + [{mode:"source-subtree", source:$source, target:$target}]' \
          "$deliveries_file" > "$temporary_json"
        mv "$temporary_json" "$deliveries_file"
        temporary_json="$files_file.tmp"
        jq --arg file "$witness_relative" '. + [$file] | unique' "$files_file" > "$temporary_json"
        mv "$temporary_json" "$files_file"
        ;;
      *)
        printf 'unsupported delivery mode %s\n' "$mode" >&2
        exit 1
        ;;
    esac
  done

  jq -r '.remote_examples[] | select(.mode == "go-run" and .network == "none") | .package' "$manifest_path" | while IFS= read -r example_package; do
    GOWORK=off GOPROXY=direct go run "$example_package@$remote_commit"
  done

  feature=$(jq -n \
    --arg id "$feature_id" \
    --arg contract "$contract" \
    --arg verified_at "$verified_at" \
    --argjson deliveries "$(cat "$deliveries_file")" \
    --argjson files "$(cat "$files_file")" \
    --argjson checks "$checks" \
    '{id:$id, contract:$contract, deliveries:$deliveries, files:$files, verified_at:$verified_at, checks:$checks, unverified:[]}')
  temporary_lock="$consumer_root/.agent-app-features.lock.tmp"
  jq --argjson feature "$feature" '.features += [$feature]' "$lock_path" > "$temporary_lock"
  mv "$temporary_lock" "$lock_path"
  rm -f "$deliveries_file" "$files_file"
done

jq -e '.features | length > 0' "$lock_path" >/dev/null
GOWORK=off GOPROXY=direct go run "$remote_module/cmd/feature-lock@$remote_commit" validate \
  --source "$source_root" \
  --source-commit "$remote_commit" \
  --host "$consumer_root" \
  --lock "$lock_path"
