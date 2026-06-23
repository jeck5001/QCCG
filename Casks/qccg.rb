cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.7.0"
  sha256 arm:   "a674901c40394e40103ccfe04c452ea8b1889c6fd553b32a102ce9d7f1ca375d",
         intel: "13eaec345ccd4dca10e8fa1f26c464a3f4045b27458aba7e1e3022a54ed92002"

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
