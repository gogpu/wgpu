//go:build windows

// Command gles-test is an integration test for the Pure Go GLES backend.
// It creates a window, initializes OpenGL, and renders a simple triangle.
package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/gogpu/wgpu/hal/gles"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	csOwnDC       = 0x0020
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsVisible     = 0x10000000

	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmPaint   = 0x000F
	wmKeyDown = 0x0100

	swShow = 5

	vkEscape = 0x1B
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var (
	hwnd      uintptr
	running   = true
	glContext *gl.Context
	wglCtx    *wgl.Context
)

func main() {
	fmt.Println("=== GLES Backend Integration Test ===")
	fmt.Println()

	// Step 1: Initialize WGL
	fmt.Print("1. Initializing WGL... ")
	if err := wgl.Init(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 2: Create window
	fmt.Print("2. Creating window... ")
	if err := createWindow(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 3: Create WGL context
	fmt.Print("3. Creating WGL context... ")
	var err error
	wglCtx, err = wgl.NewContext(wgl.HWND(hwnd))
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 3.5: Make context current
	fmt.Print("3.5. Making context current... ")
	if err := wglCtx.MakeCurrent(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 4: Load GL functions
	fmt.Print("4. Loading GL functions... ")
	glContext = &gl.Context{}
	if err := glContext.Load(wgl.GetGLProcAddress); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 5: Query GL info
	fmt.Println()
	fmt.Println("=== OpenGL Info ===")
	vendor := glContext.GetString(gl.VENDOR)
	renderer := glContext.GetString(gl.RENDERER)
	version := glContext.GetString(gl.VERSION)
	fmt.Printf("Vendor:   %s\n", vendor)
	fmt.Printf("Renderer: %s\n", renderer)
	fmt.Printf("Version:  %s\n", version)

	// Step 6: Test GLES backend
	fmt.Println()
	fmt.Println("=== Testing GLES Backend ===")
	if err := testGLESBackend(); err != nil {
		fmt.Printf("Backend test FAILED: %v\n", err)
	} else {
		fmt.Println("Backend test PASSED")
	}

	// Step 7: Render loop
	fmt.Println()
	fmt.Println("=== Rendering ===")
	fmt.Println("Press ESC to exit...")
	fmt.Println()

	// Show window
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	// Initial render
	render()

	// Message loop with PeekMessage for responsiveness
	procPeekMessageW := user32.NewProc("PeekMessageW")
	const pmRemove = 0x0001
	const wmQuit = 0x0012

	var m msg
	for running {
		// Use PeekMessage to avoid blocking
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0,
			pmRemove,
		)
		if ret != 0 {
			// WM_QUIT means exit
			if m.Message == wmQuit {
				running = false
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		} else {
			// No messages, render a frame
			render()
			// Cap at ~60 FPS to avoid 100% CPU
			time.Sleep(16 * time.Millisecond)
		}
	}

	// Cleanup
	fmt.Println("Cleaning up...")
	if wglCtx != nil {
		wglCtx.Destroy(wgl.HWND(hwnd))
	}
	procDestroyWindow.Call(hwnd)

	fmt.Println()
	fmt.Println("=== Test Complete ===")
}

func createWindow() error {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className := syscall.StringToUTF16Ptr("GLESTestWindow")
	windowTitle := syscall.StringToUTF16Ptr("GLES Backend Test")

	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		Style:     csOwnDC,
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  hInstance,
		ClassName: className,
	}

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		return fmt.Errorf("RegisterClassExW failed: %v", err)
	}

	style := uint32(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox)

	hwnd, _, err = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		uintptr(style),
		100, 100, 800, 600, // x, y, width, height
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %v", err)
	}

	return nil
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmClose:
		running = false
		procPostQuitMessage.Call(0)
		return 0

	case wmDestroy:
		running = false
		procPostQuitMessage.Call(0)
		return 0

	case wmPaint:
		render()
		return 0

	case wmKeyDown:
		if wParam == vkEscape {
			running = false
			procPostQuitMessage.Call(0)
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func render() {
	if glContext == nil || wglCtx == nil {
		return
	}

	// Ensure GL context is current (only if not already current)
	if wgl.GetCurrentContext() != wglCtx.HGLRC() {
		if err := wglCtx.MakeCurrent(); err != nil {
			return
		}
	}

	// Clear to cornflower blue
	glContext.ClearColor(0.392, 0.584, 0.929, 1.0)
	glContext.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	// Swap buffers
	_ = wglCtx.SwapBuffers()
}

func testGLESBackend() error {
	// Test 1: Create backend
	fmt.Print("  Creating backend... ")
	backend := gles.Backend{}
	fmt.Printf("OK (variant: %v)\n", backend.Variant())

	// Test 2: Create instance
	fmt.Print("  Creating instance... ")
	instance, err := backend.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("CreateInstance: %w", err)
	}
	fmt.Println("OK")

	// Test 3: Create surface
	fmt.Print("  Creating surface... ")
	surface, err := instance.CreateSurface(0, hwnd)
	if err != nil {
		return fmt.Errorf("CreateSurface: %w", err)
	}
	fmt.Println("OK")

	// Test 4: Enumerate adapters
	fmt.Print("  Enumerating adapters... ")
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		return fmt.Errorf("no adapters found")
	}
	fmt.Printf("OK (found %d)\n", len(adapters))

	// Print adapter info
	for i, exposed := range adapters {
		fmt.Printf("    Adapter %d: %s (%s)\n", i, exposed.Info.Name, exposed.Info.Driver)
	}

	// Test 5: Create device
	fmt.Print("  Creating device... ")
	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		return fmt.Errorf("Open: %w", err)
	}
	device := openDev.Device
	fmt.Println("OK")

	// Test 6: Create command encoder
	fmt.Print("  Creating command encoder... ")
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("CreateCommandEncoder: %w", err)
	}
	fmt.Println("OK")

	// Test 7: Begin/End encoding
	fmt.Print("  Testing command encoding... ")
	if err := encoder.BeginEncoding("test"); err != nil {
		return fmt.Errorf("BeginEncoding: %w", err)
	}
	cmdBuf, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("EndEncoding: %w", err)
	}
	cmdBuf.Destroy()
	fmt.Println("OK")

	// Cleanup
	device.Destroy()
	surface.Destroy()
	instance.Destroy()

	return nil
}
