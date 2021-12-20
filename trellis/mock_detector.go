package trellis

type MockProjectDetector struct {
	detected bool
}

func (p *MockProjectDetector) Detect(path string) (projectPath string, ok bool) {
	return "trellis", p.detected
}
