package ansible

import (
	"slices"
	"testing"
)

func TestParseTags(t *testing.T) {
	tagsOutput := `
playbook: dev.yml

	play #1 (web:&development): WordPress Server: Install LEMP Stack with PHP and MariaDB MySQL	TAGS: []
	    TASK TAGS: [common, composer]
	`

	expectedTags := []string{
		"common",
		"composer",
	}

	tags := ParseTags(tagsOutput)

	if !slices.Equal(tags, expectedTags) {
		t.Errorf("ParseTags() = %v, want %v", tags, expectedTags)
	}
}
