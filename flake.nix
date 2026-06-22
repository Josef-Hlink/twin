{
  description = "twin — dev environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-linux" "aarch64-linux" "x86_64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
    in {
      devShells = forAllSystems (pkgs:
        let
          go = pkgs.go_1_25;   # matches `go 1.25.4` in go.mod (unstable default `go` is 1.26)

          # gofmt + go vet gate, run against staged Go files before each commit.
          # Uses absolute store paths so it works even when invoked outside the shell.
          preCommit = pkgs.writeShellScript "twin-pre-commit" ''
            set -euo pipefail
            files=$(git diff --cached --name-only --diff-filter=ACM -- '*.go')
            if [ -n "$files" ]; then
              unformatted=$(${go}/bin/gofmt -l $files)
              if [ -n "$unformatted" ]; then
                echo "gofmt: staged Go files need formatting:" >&2
                echo "$unformatted" >&2
                echo "fix with: gofmt -w $unformatted" >&2
                exit 1
              fi
            fi
            ${go}/bin/go vet ./...
          '';
        in {
          default = pkgs.mkShell {
            packages = [
              go
              pkgs.gopls     # editor LSP
              pkgs.gotools   # goimports & friends
            ];

            # Symlink the pre-commit hook in on shell entry; refreshed each time
            # so it tracks the pinned toolchain. Skipped outside a git checkout.
            shellHook = ''
              if [ -d .git ]; then
                ln -sf ${preCommit} .git/hooks/pre-commit
              fi
            '';
          };
        });
    };
}
