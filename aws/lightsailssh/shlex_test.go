package main

import "fmt"

func Example_shJoin() {
	fmt.Println(shJoin([]string{
		``,
		`such/safe/123`,
		`$'b`,
	}))

	// Output:
	// '' such/safe/123 '$'"'"'b'
}
