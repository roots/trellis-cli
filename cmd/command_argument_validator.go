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
		return fmt.Errorf("Error: too many arguments (expected %s, got %d)\n", expectedCount, argCount)
	}
	if argCount < c.required {
		return fmt.Errorf("Error: missing arguments (expected %s, got %d)\n", expectedCount, argCount)
	}

	return nil
}
