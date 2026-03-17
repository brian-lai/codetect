# Homebrew formula for codetect
#
# This file is the reference formula. The canonical, published version lives in
# the homebrew tap repo: https://github.com/brian-lai/homebrew-tap
#
# To install:
#   brew tap brian-lai/tap
#   brew install codetect
#
# To update this formula after a new release, update `url`, `sha256`, and `version`.
# The release workflow (`.github/workflows/release.yml`) should automate this.

class Codetect < Formula
  desc "Fast, token-efficient codebase search MCP server for Claude Code and any LLM"
  homepage "https://github.com/brian-lai/codetect"
  url "https://github.com/brian-lai/codetect/archive/refs/tags/v3.7.5.tar.gz"
  sha256 "PLACEHOLDER_SHA256_UPDATE_ON_RELEASE"
  license "MIT"
  version "3.7.5"

  depends_on "go" => :build
  depends_on "ripgrep"

  def install
    system "make", "build"
    bin.install "dist/codetect" => "codetect-mcp"
    bin.install "dist/codetect-index"
    bin.install "dist/codetect-daemon"
    bin.install "dist/codetect-eval"
    bin.install "scripts/codetect-wrapper.sh" => "codetect"
    (share/"codetect/templates").install Dir["templates/*"]
    # Write VERSION file for non-git version reporting
    (share/"codetect/VERSION").write(version.to_s + "\n")
    # Copy the binary installer for `codetect update`
    (share/"codetect").install "scripts/install-binary.sh"
  end

  def post_install
    config_dir = Pathname.new(ENV["HOME"]) / ".config/codetect"
    config_dir.mkpath
    (config_dir / "install_method").write("brew\n")
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/codetect version")
  end
end
