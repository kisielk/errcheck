package assert

func main() {
	var i interface{}
	_ = i.(string) // want "unchecked error"
}
