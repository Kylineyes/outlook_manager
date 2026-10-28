package main

import "fmt"

type cliExitError struct {
	code int
	err  error
}

func (e *cliExitError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *cliExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *cliExitError) ExitCode() int {
	if e == nil || e.code == 0 {
		return 1
	}
	return e.code
}

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &cliExitError{code: code, err: err}
}

func usageError(format string, args ...any) error {
	return withExitCode(2, fmt.Errorf(format, args...))
}
