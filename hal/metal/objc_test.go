// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

func TestObjCRuntimeBasics(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pool := NewAutoreleasePool()
	if pool == nil || pool.pool == 0 {
		t.Fatal("NewAutoreleasePool returned nil")
	}
	defer pool.Drain()

	nsObject := GetClass("NSObject")
	if nsObject == 0 {
		t.Fatal("GetClass(NSObject) returned nil")
	}

	nsString := GetClass("NSString")
	if nsString == 0 {
		t.Fatal("GetClass(NSString) returned nil")
	}

	alloc := RegisterSelector("alloc")
	initSel := RegisterSelector("init")
	releaseSel := RegisterSelector("release")
	if alloc == 0 || initSel == 0 || releaseSel == 0 {
		t.Fatal("RegisterSelector returned nil")
	}

	value := "wgpu"
	ns := NSString(value)
	if ns == 0 {
		t.Fatal("NSString returned nil")
	}

	length := MsgSendUint(ns, Sel("length"))
	if length != uint(len(value)) {
		t.Fatalf("NSString length = %d, want %d", length, len(value))
	}

	got := GoString(ns)
	if got != value {
		t.Fatalf("GoString = %q, want %q", got, value)
	}

	ns2 := NSString(value)
	if ns2 == 0 {
		t.Fatal("NSString second value returned nil")
	}

	if !MsgSendBool(ns, Sel("isEqualToString:"), uintptr(ns2)) {
		t.Fatal("NSString isEqualToString returned false")
	}

	Release(ns2)
	Release(ns)

	obj := MsgSend(ID(nsObject), alloc)
	if obj == 0 {
		t.Fatal("NSObject alloc returned nil")
	}
	obj = MsgSend(obj, initSel)
	if obj == 0 {
		t.Fatal("NSObject init returned nil")
	}
	_ = MsgSend(obj, releaseSel)
}

func TestAutoreleasePoolHelperNormal(t *testing.T) {
	var events []string
	pool := newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
		lock: func() { events = append(events, "lock") },
		unlock: func() {
			events = append(events, "unlock")
		},
		create: func() ID {
			events = append(events, "create")
			return 41
		},
		drain: func(id ID) { events = append(events, "drain:"+fmt.Sprint(id)) },
	})

	if pool.pool != 41 || !pool.locked {
		t.Fatalf("constructed pool = %#v, want live pool 41", pool)
	}
	pool.Drain()
	pool.Drain()
	if pool.pool != 0 || pool.locked {
		t.Fatalf("drained pool = %#v, want terminal state", pool)
	}
	if got, want := fmt.Sprint(events), "[lock create drain:41 unlock]"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestAutoreleasePoolHelperZeroPoolStillUnlocks(t *testing.T) {
	unlockCalls := 0
	drainCalls := 0
	pool := newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
		lock:   func() {},
		unlock: func() { unlockCalls++ },
		create: func() ID { return 0 },
		drain:  func(ID) { drainCalls++ },
	})

	pool.Drain()
	if unlockCalls != 1 {
		t.Fatalf("unlock calls = %d, want 1", unlockCalls)
	}
	if drainCalls != 0 {
		t.Fatalf("drain calls = %d, want 0", drainCalls)
	}
}

func TestAutoreleasePoolHelperDoubleDrainAndNilAreNoOps(t *testing.T) {
	unlockCalls := 0
	drainCalls := 0
	pool := newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
		lock:   func() {},
		unlock: func() { unlockCalls++ },
		create: func() ID { return 9 },
		drain:  func(ID) { drainCalls++ },
	})

	pool.Drain()
	pool.Drain()
	var nilPool *AutoreleasePool
	nilPool.Drain()
	if unlockCalls != 1 || drainCalls != 1 {
		t.Fatalf("calls = unlock %d, drain %d; want 1, 1", unlockCalls, drainCalls)
	}
}

func TestAutoreleasePoolHelperNestedBalances(t *testing.T) {
	depth := 0
	creates := 0
	var events []string
	callbacks := autoreleasePoolCallbacks{
		lock: func() {
			depth++
			events = append(events, fmt.Sprintf("lock:%d", depth))
		},
		unlock: func() {
			events = append(events, fmt.Sprintf("unlock:%d", depth))
			depth--
		},
		create: func() ID {
			creates++
			events = append(events, fmt.Sprintf("create:%d", creates))
			return ID(creates)
		},
		drain: func(id ID) { events = append(events, fmt.Sprintf("drain:%d", id)) },
	}

	outer := newAutoreleasePoolWithCallbacks(callbacks)
	inner := newAutoreleasePoolWithCallbacks(callbacks)
	if depth != 2 {
		t.Fatalf("nested lock depth = %d, want 2", depth)
	}
	inner.Drain()
	outer.Drain()
	if depth != 0 {
		t.Fatalf("final lock depth = %d, want 0", depth)
	}
	if got, want := fmt.Sprint(events), "[lock:1 create:1 lock:2 create:2 drain:2 unlock:2 drain:1 unlock:1]"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestAutoreleasePoolHelperCreatePanicRollsBackLock(t *testing.T) {
	depth := 0
	unlockCalls := 0
	got := recoverAutoreleasePoolPanic(func() {
		newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
			lock:   func() { depth++ },
			unlock: func() { depth--; unlockCalls++ },
			create: func() ID { panic("create panic") },
			drain:  func(ID) {},
		})
	})
	if got != "create panic" {
		t.Fatalf("panic = %v, want create panic", got)
	}
	if depth != 0 || unlockCalls != 1 {
		t.Fatalf("rollback = depth %d, unlock calls %d; want 0, 1", depth, unlockCalls)
	}
}

func TestAutoreleasePoolHelperDeferredDrainCleansUpCallerPanic(t *testing.T) {
	depth := 0
	var events []string
	got := recoverAutoreleasePoolPanic(func() {
		pool := newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
			lock: func() {
				depth++
				events = append(events, "lock")
			},
			unlock: func() {
				events = append(events, "unlock")
				depth--
			},
			create: func() ID {
				events = append(events, "create")
				return 55
			},
			drain: func(id ID) {
				events = append(events, "drain:"+fmt.Sprint(id))
			},
		})
		defer pool.Drain()

		panic("caller panic")
	})

	if got != "caller panic" {
		t.Fatalf("panic = %v, want caller panic", got)
	}
	if depth != 0 {
		t.Fatalf("lock depth = %d, want 0", depth)
	}
	if got, want := fmt.Sprint(events), "[lock create drain:55 unlock]"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestAutoreleasePoolHelperDrainPanicStillUnlocksAndTerminates(t *testing.T) {
	depth := 0
	unlockCalls := 0
	drainCalls := 0
	pool := newAutoreleasePoolWithCallbacks(autoreleasePoolCallbacks{
		lock:   func() { depth++ },
		unlock: func() { depth--; unlockCalls++ },
		create: func() ID { return 77 },
		drain: func(ID) {
			drainCalls++
			panic("drain panic")
		},
	})

	got := recoverAutoreleasePoolPanic(pool.Drain)
	if got != "drain panic" {
		t.Fatalf("panic = %v, want drain panic", got)
	}
	pool.Drain()
	if pool.pool != 0 || pool.locked || depth != 0 {
		t.Fatalf("terminal pool = %#v, depth %d; want cleared and unlocked", pool, depth)
	}
	if drainCalls != 1 || unlockCalls != 1 {
		t.Fatalf("calls = drain %d, unlock %d; want 1, 1", drainCalls, unlockCalls)
	}
}

func recoverAutoreleasePoolPanic(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()
	return nil
}

func TestAutoreleasePoolPinsAndBalancesOSThread(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	const workers = 8
	const iterations = 128
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				outer := NewAutoreleasePool()
				if outer == nil || !outer.locked {
					t.Errorf("outer pool did not pin its goroutine")
					return
				}
				inner := NewAutoreleasePool()
				if inner == nil || !inner.locked {
					t.Errorf("inner pool did not pin its goroutine")
					outer.Drain()
					return
				}
				// Exercise goffi calls while the nested pool owns the thread. A
				// scheduler yield must not migrate this goroutine before Drain.
				if GetClass("NSObject") == 0 || GetClass("NSString") == 0 {
					t.Errorf("ObjC class lookup failed")
				}
				runtime.Gosched()
				inner.Drain()
				inner.Drain()
				if inner.locked {
					t.Errorf("inner pool remained pinned after Drain")
				}
				outer.Drain()
				outer.Drain()
				if outer.locked {
					t.Errorf("outer pool remained pinned after Drain")
				}
			}
		}()
	}
	wait.Wait()
}

func TestAutoreleasePoolMetalResourceStress(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	device := CreateSystemDefaultDevice()
	if device == 0 {
		t.Skip("no Metal device available")
	}
	defer Release(device)

	const workers = 8
	const iterations = 256
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				pool := NewAutoreleasePool()
				buffer := MsgSend(device, Sel("newBufferWithLength:options:"), uintptr(4096), uintptr(MTLResourceStorageModeShared))
				if buffer == 0 {
					t.Errorf("newBufferWithLength returned nil")
					pool.Drain()
					return
				}
				Release(buffer)
				runtime.Gosched()
				pool.Drain()
				pool.Drain()
			}
		}()
	}
	wait.Wait()
}

func TestMetalDeviceQueries(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	devices := CopyAllDevices()
	if len(devices) == 0 {
		t.Skip("no Metal devices available")
	}
	defer func() {
		for _, device := range devices {
			Release(device)
		}
	}()

	for _, device := range devices {
		name := DeviceName(device)
		if name == "" {
			t.Fatal("DeviceName returned empty string")
		}

		_ = DeviceSupportsFamily(device, MTLGPUFamilyMetal3)
		_ = DeviceRegistryID(device)
		_ = DeviceIsLowPower(device)
		_ = DeviceIsHeadless(device)
		_ = DeviceIsRemovable(device)

		maxBuf := DeviceMaxBufferLength(device)
		if maxBuf == 0 {
			t.Fatal("DeviceMaxBufferLength returned 0")
		}

		_ = DeviceRecommendedMaxWorkingSetSize(device)
	}
}

func TestCAMetalLayerDrawableSize(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	device := CreateSystemDefaultDevice()
	if device == 0 {
		t.Skip("no Metal device available")
	}
	defer Release(device)

	pool := NewAutoreleasePool()
	defer pool.Drain()

	layer := MsgSend(ID(GetClass("CAMetalLayer")), Sel("new"))
	if layer == 0 {
		t.Fatal("CAMetalLayer new returned nil")
	}
	defer Release(layer)

	_ = MsgSend(layer, Sel("setDevice:"), uintptr(device))
	_ = MsgSend(layer, Sel("setPixelFormat:"), uintptr(MTLPixelFormatBGRA8Unorm))

	expected := CGSize{Width: 64, Height: 32}
	msgSendCGSize(layer, Sel("setDrawableSize:"), expected)

	got := msgSendCGSizeReturn(t, layer, Sel("drawableSize"))
	if math.Abs(float64(got.Width-expected.Width)) > 1e-6 || math.Abs(float64(got.Height-expected.Height)) > 1e-6 {
		t.Fatalf("drawableSize = %+v, want %+v", got, expected)
	}
}

func TestRenderPassDescriptorClearColor(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	// renderPassDescriptor is a factory method (+0 autoreleased). The
	// autorelease pool owns the lifetime; an explicit Release would
	// over-release and crash when the pool later tries to drain.
	desc := MsgSend(ID(GetClass("MTLRenderPassDescriptor")), Sel("renderPassDescriptor"))
	if desc == 0 {
		t.Fatal("MTLRenderPassDescriptor renderPassDescriptor returned nil")
	}

	attachments := MsgSend(desc, Sel("colorAttachments"))
	if attachments == 0 {
		t.Fatal("colorAttachments returned nil")
	}
	attachment := MsgSend(attachments, Sel("objectAtIndexedSubscript:"), 0)
	if attachment == 0 {
		t.Fatal("color attachment returned nil")
	}

	expected := MTLClearColor{Red: 0.1, Green: 0.2, Blue: 0.3, Alpha: 1.0}
	msgSendClearColor(attachment, Sel("setClearColor:"), expected)

	got := msgSendClearColorReturn(t, attachment, Sel("clearColor"))
	if math.Abs(got.Red-expected.Red) > 1e-6 || math.Abs(got.Green-expected.Green) > 1e-6 || math.Abs(got.Blue-expected.Blue) > 1e-6 || math.Abs(got.Alpha-expected.Alpha) > 1e-6 {
		t.Fatalf("clearColor = %+v, want %+v", got, expected)
	}
}

func msgSendCGSizeReturn(t *testing.T, obj ID, sel SEL) CGSize {
	t.Helper()
	if obj == 0 || sel == 0 {
		t.Fatal("msgSendCGSizeReturn requires non-nil object and selector")
	}
	var result CGSize
	if err := msgSend(obj, sel, cgSizeType, unsafe.Pointer(&result)); err != nil {
		t.Fatalf("msgSend failed: %v", err)
	}
	return result
}

func msgSendClearColorReturn(t *testing.T, obj ID, sel SEL) MTLClearColor {
	t.Helper()
	if obj == 0 || sel == 0 {
		t.Fatal("msgSendClearColorReturn requires non-nil object and selector")
	}
	var result MTLClearColor
	if err := msgSend(obj, sel, mtlClearColorType, unsafe.Pointer(&result)); err != nil {
		t.Fatalf("msgSend failed: %v", err)
	}
	return result
}
