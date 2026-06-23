cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.7.0"
  sha256 arm:   "58ffe636a9dd43569f3e762ba527fdcb73b2b8232736be6352d10a0038597fdd",
         intel: "cf6eca6f48d56d177a039157da77125049fe025c0815b24e7b377aa3f783777d"

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
