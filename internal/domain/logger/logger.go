/*
[2026-07-02] :: 🚀 :: Initial logger package
*/

// Package logger provides a structured logging interface with slog adapter.
package logger

import (
	"time"
)

// Logger is the abstract interface for structured logging.
type Logger interface {
	Debug(pkg, msg string, fields ...Field)
	Info(pkg, msg string, fields ...Field)
	Warn(pkg, msg string, fields ...Field)
	Error(pkg, msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Field is a key-value pair for structured logging.
type Field struct {
	Key   string
	Value any
}

// ImportanceKey is the field key for importance (1-10 scale).
const ImportanceKey = "imp"

// FunctionKey is the field key for function name.
const FunctionKey = "func"

// BlockKey is the field key for block name.
const BlockKey = "block"

// StatusKey is the field key for status.
const StatusKey = "status"

// ErrorKey is the field key for error.
const ErrorKey = "error"

// String creates a string log field.
func String(key, value string) Field { return Field{Key: key, Value: value} }

// Int creates an int log field.
func Int(key string, value int) Field { return Field{Key: key, Value: value} }

// Bool creates a bool log field.
func Bool(key string, value bool) Field { return Field{Key: key, Value: value} }

// Time creates a time.Time log field.
func Time(key string, value time.Time) Field { return Field{Key: key, Value: value} }

// Error creates an error log field.
func Error(err error) Field { return Field{Key: ErrorKey, Value: err} }

// Any creates an any-type log field.
func Any(key string, value any) Field { return Field{Key: key, Value: value} }

// Importance creates an importance (1-10) log field.
func Importance(v int) Field { return Field{Key: ImportanceKey, Value: v} }

// Function creates a function name log field.
func Function(name string) Field { return Field{Key: FunctionKey, Value: name} }

// Block creates a block name log field.
func Block(name string) Field { return Field{Key: BlockKey, Value: name} }

// Status creates a status log field.
func Status(s string) Field { return Field{Key: StatusKey, Value: s} }
