//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/gogpu/wgpu"
)

func main() {
	log := func(msg string) {
		js.Global().Get("document").Call("getElementById", "log").Set("innerHTML",
			js.Global().Get("document").Call("getElementById", "log").Get("innerHTML").String()+msg+"<br>")
		fmt.Println(msg)
	}

	log("wgpu browser smoke test starting...")

	// Phase 1: Instance
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		log("FAIL: CreateInstance: " + err.Error())
		return
	}
	log("OK: Instance created")

	// Phase 1: Adapter
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		log("FAIL: RequestAdapter: " + err.Error())
		return
	}
	log(fmt.Sprintf("OK: Adapter — %s", adapter.Info().Name))
	log(fmt.Sprintf("    Features: %d", adapter.Features()))

	// Phase 1: Device
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		log("FAIL: RequestDevice: " + err.Error())
		return
	}
	log("OK: Device created")
	log(fmt.Sprintf("    MaxBufferSize: %d", device.Limits().MaxBufferSize))

	// Phase 2: Buffer
	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "test-buffer",
		Size:  256,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		log("FAIL: CreateBuffer: " + err.Error())
		return
	}
	log(fmt.Sprintf("OK: Buffer created (size=%d)", buffer.Size()))
	buffer.Release()

	// Phase 2: ShaderModule
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "test-shader",
		WGSL:  "@vertex fn vs() -> @builtin(position) vec4f { return vec4f(0,0,0,1); }",
	})
	if err != nil {
		log("FAIL: CreateShaderModule: " + err.Error())
		return
	}
	log("OK: ShaderModule created")
	shader.Release()

	// Phase 3: CommandEncoder + Submit
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		log("FAIL: CreateCommandEncoder: " + err.Error())
		return
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		log("FAIL: Finish: " + err.Error())
		return
	}
	_, err = device.Queue().Submit(cmdBuf)
	if err != nil {
		log("FAIL: Submit: " + err.Error())
		return
	}
	log("OK: Empty command buffer submitted")

	// Phase 3: WriteBuffer
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	writeBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "write-test",
		Size:  64,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		log("FAIL: CreateBuffer for write: " + err.Error())
		return
	}
	device.Queue().WriteBuffer(writeBuf, 0, data)
	log("OK: WriteBuffer (8 bytes)")
	writeBuf.Release()

	device.Release()
	adapter.Release()
	instance.Release()

	log("")
	log("ALL TESTS PASSED — browser WebGPU backend works!")
}
