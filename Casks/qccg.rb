cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.5"
  sha256 arm:   "57e8fc1269578c336be8b4318a0f310b51acca44cc1c84070a7c48fc82b23bb6",
         intel: "df7dbf6db3bb4ada6d96a1c9e8913a0a87da99a5435c59b13dc8c2e2b207fb40"

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
