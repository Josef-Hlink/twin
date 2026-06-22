{
  description = "twin — dev environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-linux" "aarch64-linux" "x86_64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
    in {
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go_1_25   # matches `go 1.25.4` in go.mod (unstable default `go` is 1.26)
            pkgs.gopls     # editor LSP
            pkgs.gotools   # goimports & friends
          ];
        };
      });
    };
}
