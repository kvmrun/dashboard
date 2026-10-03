#!/bin/bash
set -eu

# Build a .deb package from the pre-built dashboard binary.
#
# The binary is produced by the regular build (make build); this script only
# stages the FHS tree and assembles the package with dpkg-deb, so no
# compiler toolchain is required in the build environment. Runtime
# dependencies are generated with dpkg-shlibdeps (the same tool
# dpkg-buildpackage uses for ${shlibs:Depends}).

declare -r PKG="kvmrun-dashboard"
declare -r BIN="bin/dashboard"
declare -r SVC="contrib/kvmrun-dashboard.service"

for f in "$BIN" "$SVC" "debian/control" "debian/copyright" "internal/version/version.go"; do
	if [[ ! -f "$f" ]]; then
		echo "Not found: ./$f" >&2
		exit 1
	fi
done
[[ -x "$BIN" ]] || { echo "Not executable: ./$BIN (run 'make build' first)" >&2; exit 1; }
command -v dpkg-shlibdeps >/dev/null 2>&1 || {
	echo "dpkg-shlibdeps not found (install the dpkg-dev package)" >&2
	exit 1
}

# The version is taken from the Go source of truth.
declare -r VER="$(sed -n 's/^[[:space:]]*Dashboard = "\([^"]*\)".*/\1/p' internal/version/version.go | head -n1)"
[[ -n "$VER" ]] || { echo "Failed to read version from internal/version/version.go" >&2; exit 1; }

declare -r ARCH="$(dpkg --print-architecture)"
declare -r RESULT_DIR="$(pwd)/packages"
declare -r OWN="$(stat --printf '%u:%g' debian)"
declare -r STAGING="$(mktemp -d)"
declare -r SHLIBDIR="$(mktemp -d)"

trap 'chown -R "$OWN" "$RESULT_DIR"; rm -rf "$STAGING" "$SHLIBDIR"' EXIT

# Stage the FHS tree.
install -d \
	"$STAGING/DEBIAN" \
	"$STAGING/usr/lib/$PKG" \
	"$STAGING/lib/systemd/system" \
	"$STAGING/usr/share/doc/$PKG"

install -m 0755 "$BIN" "$STAGING/usr/lib/$PKG/dashboard"
install -m 0644 "$SVC" "$STAGING/lib/systemd/system/$PKG.service"
install -m 0644 "debian/copyright" "$STAGING/usr/share/doc/$PKG/copyright"

# Runtime dependencies via dpkg-shlibdeps: it resolves the binary's NEEDED
# libraries to runtime packages through their symbols/shlibs files, so
# -dev packages can never leak into Depends. It needs a Source stanza in
# debian/control in the working directory.
declare -r SUBSTVARS="$SHLIBDIR/substvars"
mkdir -p "$SHLIBDIR/debian"
printf 'Source: %s\nArchitecture: %s\n' "$PKG" "$ARCH" > "$SHLIBDIR/debian/control"
: > "$SUBSTVARS"
(
	cd "$SHLIBDIR"
	dpkg-shlibdeps -T"$SUBSTVARS" "$STAGING/usr/lib/$PKG/dashboard"
)
declare -r DEPS="$(sed -n 's/^shlibs:Depends=//p' "$SUBSTVARS")"
[[ -n "$DEPS" ]] || { echo "dpkg-shlibdeps produced no dependencies" >&2; exit 1; }

# Installed size (KiB) of the staged payload.
declare -r SIZE="$(du -ks "$STAGING" | cut -f1)"

# Render the control file from the template.
declare -r CONTROL="$(
	sed \
		-e "s/@VERSION@/$VER/g" \
		-e "s/@ARCH@/$ARCH/g" \
		-e "s/@INSTALLED_SIZE@/$SIZE/g" \
		"debian/control"
)"
printf '%s\n' "${CONTROL//@DEPENDS@/$DEPS}" > "$STAGING/DEBIAN/control"

# md5sums of every payload file.
(
	cd "$STAGING"
	find . -type f -not -path './DEBIAN/*' -printf '%P\n' |
		sort |
		xargs -r md5sum > DEBIAN/md5sums
)

dpkg-deb --build --root-owner-group "$STAGING" "$RESULT_DIR"

