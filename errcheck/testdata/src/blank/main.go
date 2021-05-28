package blank

func a() error {
	return nil
}

func main() {
	_ = a() // want "unchecked error"
}
