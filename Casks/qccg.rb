cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.3"
  sha256 arm:   "0a91a5da0a3d0a4b7c2059e91d8b1f4b98be686fd79419c5e26d61b5ce0432ad",
         intel: "8b4d33b71dd9bca9af939fb12316403567de84d8975007da7526b9be66d70364"

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
