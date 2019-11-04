package cmd

import "fmt"

type CommandArgumentValidator struct {
	required int
	optional int
}

func (c *CommandArgumentValidator) validate(args []string) (err error) {
	argCount := len(args)
	totalArgs := c.required + c.optional

	expectedCount := fmt.Sprintf("exactly %d", c.required)
	if (c.optional > 0) {
		expectedCount = fmt.Sprintf("between %d and %d", c.required, totalArgs)
	}

	if argCount > totalArgs {
		err = fmt.Errorf("Error: too many arguments (expected %s, got %d)\n", expectedCount, len(args))
	} else if argCount < c.required {
		err = fmt.Errorf("Error: missing arguments (expected %s, got %d)\n", expectedCount, len(args))
	}

	return
}
