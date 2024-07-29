package lima

var UbuntuImages = map[string][]Image{
	"18.04": {
		{
			Alias:    "bionic",
			Location: "https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-amd64.img",
			Arch:     "x86_64",
		},
		{
			Alias:    "bionic",
			Location: "https://cloud-images.ubuntu.com/bionic/current/bionic-server-cloudimg-arm64.img",
			Arch:     "aarch64",
		},
	},
	"20.04": {
		{
			Alias:    "focal",
			Location: "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img",
			Arch:     "x86_64",
		},
		{
			Alias:    "focal",
			Location: "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-arm64.img",
			Arch:     "aarch64",
		},
	},
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
