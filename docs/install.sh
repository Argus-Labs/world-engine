#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

set -e

main() {
	os=$(uname -s)
	arch=$(uname -m)
	version=${1:-latest}

	if [ "$version" = "latest" ]; then
		release_url="https://api.github.com/repos/Argus-Labs/world-cli/releases/latest"
	else
		release_url="https://api.github.com/repos/Argus-Labs/world-cli/releases/tags/$version"
	fi

	download_url=$(curl -s "$release_url" \
		| grep "browser_download_url.*world-cli_${os}_${arch}.tar.gz" \
		| cut -d '"' -f 4)
	if [ -z "$download_url" ]; then
		echo "Error: No binary found for $os/$arch/$version - see github.com/Argus-Labs/world-cli/releases for all versions" 1>&2
		exit 1
	fi

	world_install="${WORLD_INSTALL:-$HOME/.worldcli}"

	bin_dir="$world_install/bin"
	tmp_dir="$world_install/tmp"
	exe="$bin_dir/world"

	mkdir -p "$bin_dir"
	mkdir -p "$tmp_dir"

	curl -q --fail --location --progress-bar --output "$tmp_dir/world.tar.gz" "$download_url"
	# extract to tmp dir so we don't open existing executable file for writing:
	tar -C "$tmp_dir" -xzf "$tmp_dir/world.tar.gz"
	chmod +x "$tmp_dir/world"
	# atomically rename into place:
	mv "$tmp_dir/world" "$exe"
	rm "$tmp_dir/world.tar.gz"

	echo "world was installed successfully to $exe"
	if command -v world >/dev/null; then
		echo "Run 'world --help' to get started"
	else
		case $SHELL in
		/bin/zsh)
			if [ -f "$HOME/.zshrc" ] && ! grep -q 'WORLD_INSTALL' "$HOME/.zshrc"; then
				printf '\nexport WORLD_INSTALL="%s"\nexport PATH="$WORLD_INSTALL/bin:$PATH"\n' "$world_install" >> "$HOME/.zshrc"
				echo "Restart your terminal or run:\n  source ~/.zshrc"
			fi
			;;
		*/bash)
			if [ -f "$HOME/.bashrc" ] && ! grep -q 'WORLD_INSTALL' "$HOME/.bashrc"; then
				printf '\nexport WORLD_INSTALL="%s"\nexport PATH="$WORLD_INSTALL/bin:$PATH"\n' "$world_install" >> "$HOME/.bashrc"
				echo "Restart your terminal or run:\n  source ~/.bashrc"
			fi
			;;
		*)
			echo "Manually add the directory to your shell profile (e.g. ~/.profile)"
			echo "  export WORLD_INSTALL=\"$world_install\""
			echo "  export PATH=\"\$WORLD_INSTALL/bin:\$PATH\""
			;;
		esac
		echo "Run 'world --help' to get started"
	fi
}

main "$1"
