//go:build js && wasm

package browser

import (
	"syscall/js"
)

// AwaitPromise blocks the calling goroutine until a JS Promise resolves or rejects.
//
// On WASM, Go goroutines are cooperative (single-threaded). This function uses
// Promise.then/catch with a channel to yield the goroutine until the JS event
// loop resolves the promise. The caller MUST be on a goroutine (not the main
// goroutine) or the program will deadlock.
//
// Pattern matches Rust wgpu's wasm_bindgen_futures::JsFuture::from(promise).
func AwaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan promiseResult, 1)

	thenFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var val js.Value
		if len(args) > 0 {
			val = args[0]
		} else {
			val = js.Undefined()
		}
		ch <- promiseResult{value: val}
		return nil
	})

	catchFn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var val js.Value
		if len(args) > 0 {
			val = args[0]
		} else {
			val = js.Undefined()
		}
		ch <- promiseResult{value: val, rejected: true}
		return nil
	})

	promise.Call("then", thenFn).Call("catch", catchFn)

	result := <-ch

	// Release JS functions after promise settles to prevent GC leaks.
	thenFn.Release()
	catchFn.Release()

	if result.rejected {
		return js.Undefined(), &PromiseRejection{Value: result.value}
	}

	return result.value, nil
}

// PromiseRejection carries the JS rejection value of a settled promise, so a
// caller can inspect it (name, constructor) instead of parsing a message
// string. It implements error, so existing callers treat it as before.
type PromiseRejection struct {
	// Value is the JS rejection value (a JS Error object in practice).
	Value js.Value
}

// Error implements the error interface.
func (e *PromiseRejection) Error() string {
	return "promise rejected: " + extractErrorMessage(e.Value)
}

// promiseResult carries either a resolved value or a rejection.
type promiseResult struct {
	value    js.Value
	rejected bool
}

// extractErrorMessage extracts a human-readable message from a JS error value.
func extractErrorMessage(v js.Value) string {
	if v.IsUndefined() || v.IsNull() {
		return "unknown error"
	}

	// Try .message property first (standard Error objects).
	msg := v.Get("message")
	if !msg.IsUndefined() && !msg.IsNull() {
		return msg.String()
	}

	// Fall back to toString().
	return v.Call("toString").String()
}
