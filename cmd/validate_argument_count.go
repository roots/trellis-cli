package cmd

import "fmt"

func validateArgumentCount(args []string, requiredArgCount int, optionalArgCount int) (err error) {
	argCount := len(args)

	expectedCount := fmt.Sprintf("exactly %d", requiredArgCount)
	if (optionalArgCount > 0) {
		expectedCount = fmt.Sprintf("between %d and %d", requiredArgCount, requiredArgCount + optionalArgCount)
	}

	if argCount > requiredArgCount + optionalArgCount {
		err = fmt.Errorf("Error: too many arguments (expected %s, got %d)\n", expectedCount, len(args))
	} else if argCount < requiredArgCount {
		err = fmt.Errorf("Error: missing arguments (expected %s, got %d)\n", expectedCount, len(args))
	}

	return
}
