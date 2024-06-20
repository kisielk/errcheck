package assert

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
		_ = i.(int) // want "unchecked error"
	case nil:
	}
}

func handleInterface(i interface{}) string {
	return i.(string) // want "unchecked error"
}
