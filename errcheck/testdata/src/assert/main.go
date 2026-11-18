package assert

import "fmt"

func main() {
	var i interface{}
	_ = i.(string) // want "unchecked error"

	handleInterface(i.(string)) // want "unchecked error"

	if i.(string) == "hello" { // want "unchecked error"
		//
	}

	switch i.(type) {
	case string:
	case int:
		_ = i.(int)
		_ = i.(string) // want "unchecked error"
	case nil:
		_ = i.(int) // want "unchecked error"
	}

	var a interface{}
	a = 5

	switch a.(type) {
	case string:
		fmt.Println("this is string", a.(string))
	case int:
		fmt.Println("this is int", a.(int))
	default:
		fmt.Println("unknown type")
	}
}

func handleInterface(i interface{}) string {
	return i.(string) // want "unchecked error"
}
