cask "qccg" do
  arch arm: "arm64", intel: "amd64"

  version "0.6.2"
  sha256 arm:   "2edd3d0b865d49b824d81afba198a1147e077cbb5ef80eeb775f2dcb6b6edca6",
         intel: "98c47fe502fb26c32a7526e590ca4bd8155783518f9bb2fb380d06dafd0710f3"

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
