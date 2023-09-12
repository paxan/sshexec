package internal

import "fmt"

func ExampleSHJoin() {
	fmt.Println(SHJoin([]string{
		``,
		`such/safe/123`,
		`$'b`,
	}))

	// Output:
	// '' such/safe/123 '$'"'"'b'
}
