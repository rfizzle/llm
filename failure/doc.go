// Package failure is the provider failure taxonomy: a closed set of classes
// every backend's errors are classified into, with what a recovery needs to
// be offered honestly. Match a class with errors.Is against its sentinel, or
// take the detail apart with As.
package failure
