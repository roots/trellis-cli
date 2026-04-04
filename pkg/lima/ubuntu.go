package lima

var UbuntuImages = map[string][]Image{
	"22.04": {
		{
			Alias:    "jammy",
			Location: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
			Arch:     "x86_64",
		},
		{
			Alias:    "jammy",
			Location: "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-arm64.img",
			Arch:     "aarch64",
		},
	},
	"24.04": {
		{
			Alias:    "noble",
			Location: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
			Arch:     "x86_64",
		},
		{
			Alias:    "noble",
			Location: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-arm64.img",
			Arch:     "aarch64",
		},
	},
}
