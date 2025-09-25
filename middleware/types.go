// Copyright (c) 2017 Gorillalabs. All rights reserved.

package middleware

// Middleware is the interface implemented by different session types
type Middleware interface {
	Execute(cmd string) (string, string, error)
	Exit()
}
