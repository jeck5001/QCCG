cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.1"
  sha256 arm:   "e7603d7f5c61af468004c8d0ac84d1312f83ac28a8430b60325bf611dd5cb5a5",
         intel: "a2e1b9f8d3a1d7197bb7b3ec9a00f9642155c6edc3de3222d885cdfa7231fbbb"

  url "https://github.com/wangtufly/QCCG/releases/download/v#{version}/QCCG-v#{version}-darwin-#{arch}.dmg"
  name "QCCG"
  desc "Qoder Claude Codex Gemini Gateway"
  homepage "https://github.com/wangtufly/QCCG"

  depends_on macos: ">= :monterey"

  app "qccg.app", target: "QCCG.app"

  zap trash: [
    "~/Library/Application Support/QCCG",
    "~/Library/Preferences/com.qccg.app.plist",
  ]
end
