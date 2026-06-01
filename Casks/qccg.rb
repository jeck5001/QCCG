cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.4"
  sha256 arm:   "a72f0ca9d0281d04c0a8a27919d1aae100ec45121920a6927ccb228360fc3ddc",
         intel: "cdb83495050c92168ccac9a7950f1c7853f9f8a4dd51fc4496717bd37a5ad6af"

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
