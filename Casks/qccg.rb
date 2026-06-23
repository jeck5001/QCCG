cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.7.0"
  sha256 arm:   "6d21751aaf33a08d968ce2073fbca3a16b110ee8c336562559306f3c6af16220",
         intel: "5a284828fb8c1c388a2e82c080fef725ed07befe4339b79c6891aaefa5a5f785"

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
