package blank

import "fmt"

func a() error {
	return nil
}

func b() (string, error) {
	return "", nil
}

func c() string {
	return ""
}

func main() {
	_ = a() // want "unchecked error"
	a()     // want "unchecked error"
	b()     // want "unchecked error"
	c()     // ignored, doesn't return an error

	{
		r, err := b() // fine, we're checking the error
		fmt.Printf("r = %v, err = %v\n", r, err)
	}

	{
		r, _ := b() // want "unchecked error"
		fmt.Printf("r = %v\n", r)
	}

	{
		var r, _ = b() // want "unchecked error"
		fmt.Printf("r = %v\n", r)
	}
}
