package main

func testChannels() {
	errChan := make(chan error)
	customErrChan := make(chan MyError)
	ptrErrChan := make(chan *MyPointerError)
	intChan := make(chan int)

	// Standalone receives in expression statements
	<-errChan       // UNCHECKED
	(<-errChan)     // UNCHECKED
	<-customErrChan // UNCHECKED
	<-ptrErrChan    // UNCHECKED

	// Non-error channels are ignored
	<-intChan
	(<-intChan)

	// Assignments to variables are not reported
	err := <-errChan
	_ = err
	customErr := <-customErrChan
	_ = customErr
	ptrErr := <-ptrErrChan
	_ = ptrErr
	val := <-intChan
	_ = val

	err2, ok := <-errChan
	_, _ = err2, ok
	val2, ok2 := <-intChan
	_, _ = val2, ok2

	// Only ignoring the ok boolean is not an unchecked error
	err3, _ := <-errChan
	_ = err3

	// Blank assignments for error channels
	_ = <-errChan           // BLANK
	_, ok = <-errChan       // BLANK
	_, _ = <-errChan        // BLANK
	_ = <-customErrChan     // BLANK
	_, ok = <-customErrChan // BLANK
	_ = <-ptrErrChan        // BLANK
	_, ok = <-ptrErrChan    // BLANK

	// Blank assignments for non-error channels are ignored
	_ = <-intChan
	_, ok2 = <-intChan
	_, _ = <-intChan

	// Declarations with blank assignments
	var _ = <-errChan       // BLANK
	var _, ok3 = <-errChan  // BLANK
	_ = ok3
	var _ = <-customErrChan // BLANK
	var _ = <-ptrErrChan    // BLANK
	var _ = <-intChan
	var _, ok4 = <-intChan
	_ = ok4

	// Multi-value assignments
	_, x := <-errChan, 1 // BLANK
	_ = x
	e, y := <-errChan, 2
	_, _ = e, y
	_, z := <-intChan, 3
	_ = z

	// Select statements
	select {
	case <-errChan: // UNCHECKED
	case err := <-errChan:
		_ = err
	case _, ok := <-errChan: // BLANK
		_ = ok
	case <-intChan:
	case val := <-intChan:
		_ = val
	case _, ok := <-intChan:
		_ = ok
	default:
	}
}
