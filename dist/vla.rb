# Homebrew formula for VLA.
# Usage: brew tap outerstellar-hq/tap https://github.com/outerstellar-hq/vla
#        brew install vla
#
# Or directly: brew install --build-from-source ./dist/vla.rb
class Vla < Formula
  desc "CLI agentic coding harness with persistent memory and LSP-backed code intelligence"
  homepage "https://github.com/outerstellar-hq/vla"
  url "https://github.com/outerstellar-hq/vla/archive/refs/tags/v0.2.0.tar.gz"
  sha256 "REPLACE_WITH_ACTUAL_SHA256"
  license "MIT"
  head "https://github.com/outerstellar-hq/vla.git", branch: "main"

  # VLA needs Go 1.26+ for new stdlib features.
  depends_on "go" => :build

  def install
    # Build the binary with the version embedded.
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=v0.2.0")
  end

  test do
    # `vla version` should print the version string.
    assert_match "vla", shell_output("#{bin}/vla version")
  end
end
