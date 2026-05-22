cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.5.3"
  sha256 arm:   "4ecdb9756059263712710add57c76b5817092d32e7356d6a44b73ff4d4904e54",
         intel: "f45a289277fe8bb91a4438ccac2dc1a883544771711ac754ab154fb8b58b4f91"

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
