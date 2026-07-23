#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
module_state_before=$(git -C "$root" hash-object go.mod go.sum)

sha256_stream() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum
	else
		shasum -a 256
	fi
}

worktree_fingerprint() {
	local repo=$1
	{
		git -C "$repo" diff --binary --no-ext-diff HEAD -- .
		git -C "$repo" ls-files --others --exclude-standard | LC_ALL=C sort |
			while IFS= read -r file; do
				[[ -n "$file" ]] || continue
				printf 'UNTRACKED %s\n' "$file"
				if command -v sha256sum >/dev/null 2>&1; then
					sha256sum "$repo/$file" | awk '{print $1}'
				else
					shasum -a 256 "$repo/$file" | awk '{print $1}'
				fi
			done
	} | sha256_stream | awk '{print $1}'
}

require_checkout() {
	local label=$1
	local repo=$2
	local expected_head=$3
	local expected_patch=$4
	local actual_head
	local actual_patch

	actual_head=$(git -C "$repo" rev-parse HEAD)
	if [[ "$actual_head" != "$expected_head" ]]; then
		echo "$label HEAD is $actual_head, want $expected_head" >&2
		exit 1
	fi
	git -C "$repo" diff --check
	actual_patch=$(worktree_fingerprint "$repo")
	if [[ "$actual_patch" != "$expected_patch" ]]; then
		echo "$label working-tree fingerprint is $actual_patch, want $expected_patch" >&2
		exit 1
	fi
}

goffi_module=github.com/go-webgpu/goffi
goffi_version=v0.6.1
(
	cd "$root"
	GOWORK=off go mod download "$goffi_module@$goffi_version"
)

webgpu_dir=
actual_webgpu_head=
if [[ -n "${WEBGPU_DIR:-}" ]]; then
	: "${WEBGPU_EXPECTED_HEAD:?set WEBGPU_EXPECTED_HEAD to the helper source immutable commit}"
	: "${WEBGPU_EXPECTED_PATCH:?set WEBGPU_EXPECTED_PATCH to its working-tree fingerprint}"
	webgpu_dir=$(cd "$WEBGPU_DIR" && pwd -P)
	actual_webgpu_head=$(git -C "$webgpu_dir" rev-parse HEAD)
	require_checkout go-webgpu/webgpu "$webgpu_dir" "$WEBGPU_EXPECTED_HEAD" "$WEBGPU_EXPECTED_PATCH"
elif [[ -n "${WEBGPU_EXPECTED_HEAD:-}" || -n "${WEBGPU_EXPECTED_PATCH:-}" ]]; then
	echo "WEBGPU_DIR is required when a go-webgpu/webgpu identity is supplied" >&2
	exit 1
fi

: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to Android NDK r29}"
if ! grep -q '^Pkg.Revision = 29\.' "$ANDROID_NDK_HOME/source.properties"; then
	echo "Android NDK r29 is required" >&2
	exit 1
fi

readelf_path=$(find "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt" -path '*/bin/llvm-readelf' \( -type f -o -type l \) -print -quit)
clang_path=$(find "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt" -path '*/bin/aarch64-linux-android29-clang' \( -type f -o -type l \) -print -quit)
if [[ -z "$readelf_path" || -z "$clang_path" ]]; then
	echo "NDK r29 llvm-readelf or API 29 arm64 clang was not found" >&2
	exit 1
fi
"$clang_path" -std=c11 -fsyntax-only "$root/scripts/testdata/android_arm64_vulkan_abi.c"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
workspace=off
if [[ -n "$webgpu_dir" ]]; then
	(
		cd "$tmpdir"
		GOWORK=off go work init "$root" "$webgpu_dir"
	)
	workspace="$tmpdir/go.work"
fi

actual_goffi_version=$(GOWORK="$workspace" go list -m -f '{{.Version}}' "$goffi_module")
if [[ "$actual_goffi_version" != "$goffi_version" ]]; then
	echo "selected goffi version is $actual_goffi_version, want $goffi_version" >&2
	exit 1
fi
goffi_replacement=$(GOWORK="$workspace" go list -m -f '{{with .Replace}}{{.Path}} {{.Version}} {{.Dir}}{{end}}' "$goffi_module")
if [[ -n "$goffi_replacement" ]]; then
	echo "goffi must come from its canonical module release, not replacement $goffi_replacement" >&2
	exit 1
fi
if [[ -n "$webgpu_dir" ]]; then
	selected_webgpu=$(GOWORK="$workspace" go list -m -f '{{.Dir}}' github.com/go-webgpu/webgpu)
	if [[ "$(cd "$selected_webgpu" && pwd -P)" != "$webgpu_dir" ]]; then
		echo "workspace selected go-webgpu/webgpu from $selected_webgpu, want $webgpu_dir" >&2
		exit 1
	fi
fi

audit_source_selection() {
	local cgo=$1
	local vulkan_files
	local backend_files
	local dependencies="$tmpdir/android-arm64-cgo$cgo.deps"
	local -a env_args=(
		"GOWORK=$workspace"
		"GOOS=android"
		"GOARCH=arm64"
		"CGO_ENABLED=$cgo"
	)
	if [[ "$cgo" == 1 ]]; then
		env_args+=("CC=$clang_path")
	fi

	vulkan_files=$(cd "$root" && env "${env_args[@]}" go list -f '{{.GoFiles}}' ./hal/vulkan)
	backend_files=$(cd "$root" && env "${env_args[@]}" go list -f '{{.GoFiles}}' ./hal/allbackends)
	for file in api_android.go init_android.go platform_android.go; do
		grep -q "$file" <<<"$vulkan_files"
	done
	if grep -Eq 'api_linux\.go|platform_state_default\.go|(^|[^[:alnum:]_])init\.go([^[:alnum:]_]|$)' <<<"$vulkan_files"; then
		echo "Android selected a desktop Vulkan source: $vulkan_files" >&2
		exit 1
	fi
	grep -q 'register_android.go' <<<"$backend_files"
	if grep -Eq 'register(_linux)?\.go' <<<"$backend_files"; then
		echo "Android selected a desktop/fallback backend source: $backend_files" >&2
		exit 1
	fi

	(cd "$root" && env "${env_args[@]}" go list -deps ./examples/triangle-headless) >"$dependencies"
	if grep -Eq 'github.com/gogpu/wgpu/hal/(gles|software)$' "$dependencies"; then
		echo "Android dependency graph contains a GLES or software fallback" >&2
		exit 1
	fi
}

audit_elf() {
	local binary=$1
	local label=$2
	local expected_loader=$3
	local dynamic="$tmpdir/$label.dynamic"
	local strings_file="$tmpdir/$label.strings"

	"$readelf_path" -d "$binary" >"$dynamic"
	strings -a "$binary" >"$strings_file"

	grep -Eq 'Shared library: \[libc\.so\]' "$dynamic"
	grep -Eq 'Shared library: \[libdl\.so\]' "$dynamic"
	grep -q "$expected_loader" "$strings_file"

	if grep -Eq 'Shared library: \[[^]]*\.so\.[0-9]|Shared library: \[libpthread' "$dynamic"; then
		echo "$label contains a glibc-style or standalone pthread dependency" >&2
		cat "$dynamic" >&2
		exit 1
	fi
	if grep -Eq 'GLIBC_|__errno_location|libX11|libwayland-client|libEGL|libGLESv2' "$strings_file"; then
		echo "$label contains a forbidden non-Bionic/desktop symbol or library" >&2
		exit 1
	fi
}

run_mode() {
	local cgo=$1
	local label="android-arm64-cgo$cgo"
	local native_binary="$tmpdir/$label-native"
	local rust_binary="$tmpdir/$label-rust"
	local -a env_args=(
		"GOWORK=$workspace"
		"GOOS=android"
		"GOARCH=arm64"
		"CGO_ENABLED=$cgo"
	)
	if [[ "$cgo" == 1 ]]; then
		env_args+=("CC=$clang_path")
	fi

	audit_source_selection "$cgo"
	(
		cd "$root"
		env "${env_args[@]}" go test -exec=true ./...
		env "${env_args[@]}" go build -o "$native_binary" ./examples/triangle-headless
	)
	audit_elf "$native_binary" "$label-native" 'libvulkan.so'
	if [[ -n "$webgpu_dir" ]]; then
		(
			cd "$root"
			env "${env_args[@]}" go test -tags rust -exec=true ./...
			env "${env_args[@]}" go build -tags rust -o "$rust_binary" ./examples/triangle-headless
		)
		audit_elf "$rust_binary" "$label-rust" 'libwgpu_native.so'
	fi
}

run_mode 0
run_mode 1

if [[ -e "$root/go.work" || -e "$root/go.work.sum" ]]; then
	echo "preview proof must not leave a committed/local workspace in the WGPU tree" >&2
	exit 1
fi
module_state_after=$(git -C "$root" hash-object go.mod go.sum)
if [[ "$module_state_after" != "$module_state_before" ]]; then
	echo "preview proof modified go.mod or go.sum" >&2
	exit 1
fi
if [[ -n "$webgpu_dir" ]]; then
	git -C "$webgpu_dir" diff --exit-code -- go.mod go.sum
	echo "Android arm64 native and Rust checks passed with goffi $actual_goffi_version and go-webgpu/webgpu $actual_webgpu_head"
else
	echo "Android arm64 native checks passed with goffi $actual_goffi_version (Rust checks skipped: WEBGPU_DIR not set)"
fi
