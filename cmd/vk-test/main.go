//go:build windows

// Command vk-test is an integration test for the Pure Go Vulkan backend.
// It initializes Vulkan, enumerates physical devices, and creates a logical device.
//
//nolint:errcheck,gosec,staticcheck,errorlint,gocritic // test utility
package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal/vulkan"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	csOwnDC       = 0x0020
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
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

var hwnd uintptr

func main() {
	fmt.Println("=== Vulkan Backend Integration Test ===")
	fmt.Println()

	// Step 1: Initialize Vulkan library
	fmt.Print("1. Initializing Vulkan library... ")
	if err := vk.Init(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK")

	// Step 2: Load global commands
	fmt.Print("2. Loading global commands... ")
	var cmds vk.Commands
	cmds.LoadGlobal()
	fmt.Println("OK")

	// Step 3: Check vkEnumerateInstanceVersion (Vulkan 1.1+)
	fmt.Print("3. Querying Vulkan version... ")
	if proc := cmds.EnumerateInstanceVersion(); proc != 0 {
		var version uint32
		syscall.SyscallN(proc, uintptr(unsafe.Pointer(&version)))
		major := version >> 22
		minor := (version >> 12) & 0x3FF
		patch := version & 0xFFF
		fmt.Printf("OK (Vulkan %d.%d.%d)\n", major, minor, patch)
	} else {
		fmt.Println("OK (Vulkan 1.0)")
	}

	// Step 4: Create window for surface test
	fmt.Print("4. Creating window for surface... ")
	if err := createWindow(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	defer procDestroyWindow.Call(hwnd)
	fmt.Println("OK")

	// Step 5: Test Vulkan backend
	fmt.Println()
	fmt.Println("=== Testing Vulkan Backend ===")
	if err := testVulkanBackend(); err != nil {
		fmt.Printf("Backend test FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Backend test PASSED")

	fmt.Println()
	fmt.Println("=== Test Complete ===")
}

func createWindow() error {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className := syscall.StringToUTF16Ptr("VulkanTestWindow")
	windowTitle := syscall.StringToUTF16Ptr("Vulkan Backend Test")

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
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func testVulkanBackend() error {
	// Test 1: Create backend
	fmt.Print("  Creating backend... ")
	backend := vulkan.Backend{}
	fmt.Printf("OK (variant: %v)\n", backend.Variant())

	// Test 2: Create instance
	fmt.Print("  Creating instance... ")
	instance, err := backend.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("CreateInstance: %w", err)
	}
	defer instance.Destroy()
	fmt.Println("OK")

	// Test 3: Create surface
	fmt.Print("  Creating surface... ")
	surface, err := instance.CreateSurface(0, hwnd)
	if err != nil {
		return fmt.Errorf("CreateSurface: %w", err)
	}
	defer surface.Destroy()
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
		fmt.Printf("    Adapter %d: %s (%s %s)\n",
			i,
			exposed.Info.Name,
			exposed.Info.Vendor,
			exposed.Info.DriverInfo)
	}

	// Test 5: Create device
	fmt.Print("  Creating device... ")
	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		return fmt.Errorf("Open: %w", err)
	}
	device := openDev.Device
	queue := openDev.Queue
	defer device.Destroy()
	fmt.Println("OK")

	// Print queue info
	fmt.Printf("    Device and Queue created successfully\n")
	_ = queue // Queue is available for later use

	return nil
}
