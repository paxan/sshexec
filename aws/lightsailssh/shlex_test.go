package main

import "fmt"

func Example_commandJoin() {
	fmt.Println(commandJoin([]string{
		``,
		`such/safe/123`,
		`$'b`,
	}))

	// Output:
	// '' such/safe/123 '$'"'"'b'
}
