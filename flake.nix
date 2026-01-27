{
  description = "Golang flake";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      goVersion = 25; # Change this to update the whole stack

      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSupportedSystem = f: nixpkgs.lib.genAttrs supportedSystems (system: f {
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
          overlays = [ self.overlays.default ];
        };
      });

      # Build go-ai-lint from source
      go-ai-lint = { pkgs }: pkgs.buildGoModule {
        pname = "go-ai-lint";
        version = "1.0.0";
        src = pkgs.fetchFromGitHub {
          owner = "curtbushko";
          repo = "go-ai-lint";
          rev = "v1.0.0";
          sha256 = "sha256-y2G7dTZqM/rEQaALu54bHigBeO1xxRIblBJ7QxOffW4=";
        };
        subPackages = [ "cmd/go-ai-lint" ];
        vendorHash = null;
      };
    in
    {
      overlays.default = final: prev: {
        go = final."go_1_${toString goVersion}";
      };

      devShells = forEachSupportedSystem ({ pkgs }: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            docker
            # go (version is specified by overlay)
            go
            gotools
            golangci-lint
            (go-ai-lint { inherit pkgs; })
          ];
        };
      });
    };
}
