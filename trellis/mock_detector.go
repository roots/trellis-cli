package trellis

type MockProjectDetector struct {
	detected bool
}

func (p *MockProjectDetector) Detect(path string) (projectPath string, ok bool) {
	if p.detected {
		// Return current directory for mock to avoid chdir errors
		return ".", true
	}
	return "", false
}
