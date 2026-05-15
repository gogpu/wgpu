//go:build js && wasm

package browser

import "syscall/js"

// Device wraps a browser GPUDevice with pre-bound creation methods.
//
// Pre-binding JS methods at construction time avoids repeated .Get("methodName")
// calls on every frame. This pattern is used by Ebiten and other Go WASM libraries
// for optimal performance.
//
// Matches Rust wgpu WebDevice which holds the webgpu_sys::GpuDevice value.
type Device struct {
	// ref_ is the GPUDevice JavaScript object.
	ref_ js.Value

	// queue is the device's command queue (GPUQueue).
	queue *Queue

	// features is the device's GPUSupportedFeatures.
	features js.Value

	// limits is the device's GPUSupportedLimits.
	limits js.Value

	// Pre-bound creation methods. Each is the result of
	// device.methodName.bind(device), so calling fnX.Invoke(args...)
	// is equivalent to device.methodName(args...) but without the
	// property lookup overhead on each call.
	fnCreateBuffer          js.Value
	fnCreateTexture         js.Value
	fnCreateShaderModule    js.Value
	fnCreateBindGroupLayout js.Value
	fnCreateBindGroup       js.Value
	fnCreatePipelineLayout  js.Value
	fnCreateRenderPipeline  js.Value
	fnCreateComputePipeline js.Value
	fnCreateCommandEncoder  js.Value
	fnCreateSampler         js.Value
	fnCreateQuerySet        js.Value
}

// NewDevice constructs a Device from a GPUDevice js.Value.
// Pre-binds all creation methods and extracts the queue.
func NewDevice(ref js.Value) *Device {
	d := &Device{
		ref_:     ref,
		features: ref.Get("features"),
		limits:   ref.Get("limits"),
	}

	// Extract the device's queue (GPUDevice.queue is a readonly attribute).
	d.queue = NewQueue(ref.Get("queue"))

	// Pre-bind all creation methods to avoid property lookups on hot paths.
	// pattern: device.createX.bind(device) => fnCreateX
	d.fnCreateBuffer = bindMethod(ref, "createBuffer")
	d.fnCreateTexture = bindMethod(ref, "createTexture")
	d.fnCreateShaderModule = bindMethod(ref, "createShaderModule")
	d.fnCreateBindGroupLayout = bindMethod(ref, "createBindGroupLayout")
	d.fnCreateBindGroup = bindMethod(ref, "createBindGroup")
	d.fnCreatePipelineLayout = bindMethod(ref, "createPipelineLayout")
	d.fnCreateRenderPipeline = bindMethod(ref, "createRenderPipeline")
	d.fnCreateComputePipeline = bindMethod(ref, "createComputePipeline")
	d.fnCreateCommandEncoder = bindMethod(ref, "createCommandEncoder")
	d.fnCreateSampler = bindMethod(ref, "createSampler")
	d.fnCreateQuerySet = bindMethod(ref, "createQuerySet")

	return d
}

// bindMethod returns methodName.bind(obj) so that Invoke() calls the method
// with the correct `this` context.
func bindMethod(obj js.Value, methodName string) js.Value {
	method := obj.Get(methodName)
	if method.IsUndefined() || method.IsNull() {
		// Method not available — return undefined. Callers will get a
		// clear JS error when they try to invoke it.
		return js.Undefined()
	}
	return method.Call("bind", obj)
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's GPUSupportedFeatures js.Value.
func (d *Device) Features() js.Value {
	return d.features
}

// Limits returns the device's GPUSupportedLimits js.Value.
func (d *Device) Limits() js.Value {
	return d.limits
}

// Ref returns the underlying GPUDevice js.Value.
func (d *Device) Ref() js.Value {
	return d.ref_
}

// CreateBuffer returns the pre-bound createBuffer function.
// Call with: d.CreateBuffer().Invoke(descriptorObj)
func (d *Device) CreateBuffer() js.Value { return d.fnCreateBuffer }

// CreateTexture returns the pre-bound createTexture function.
func (d *Device) CreateTexture() js.Value { return d.fnCreateTexture }

// CreateShaderModule returns the pre-bound createShaderModule function.
func (d *Device) CreateShaderModule() js.Value { return d.fnCreateShaderModule }

// CreateBindGroupLayout returns the pre-bound createBindGroupLayout function.
func (d *Device) CreateBindGroupLayout() js.Value { return d.fnCreateBindGroupLayout }

// CreateBindGroup returns the pre-bound createBindGroup function.
func (d *Device) CreateBindGroup() js.Value { return d.fnCreateBindGroup }

// CreatePipelineLayout returns the pre-bound createPipelineLayout function.
func (d *Device) CreatePipelineLayout() js.Value { return d.fnCreatePipelineLayout }

// CreateRenderPipeline returns the pre-bound createRenderPipeline function.
func (d *Device) CreateRenderPipeline() js.Value { return d.fnCreateRenderPipeline }

// CreateComputePipeline returns the pre-bound createComputePipeline function.
func (d *Device) CreateComputePipeline() js.Value { return d.fnCreateComputePipeline }

// CreateCommandEncoder returns the pre-bound createCommandEncoder function.
func (d *Device) CreateCommandEncoder() js.Value { return d.fnCreateCommandEncoder }

// CreateSampler returns the pre-bound createSampler function.
func (d *Device) CreateSampler() js.Value { return d.fnCreateSampler }

// CreateQuerySet returns the pre-bound createQuerySet function.
func (d *Device) CreateQuerySet() js.Value { return d.fnCreateQuerySet }

// CreateBufferFromDesc creates a GPUBuffer from a JS descriptor object.
func (d *Device) CreateBufferFromDesc(desc js.Value) *Buffer {
	jsBuffer := d.fnCreateBuffer.Invoke(desc)
	return NewBuffer(jsBuffer)
}

// CreateTextureFromDesc creates a GPUTexture from a JS descriptor object.
func (d *Device) CreateTextureFromDesc(desc js.Value) *Texture {
	jsTexture := d.fnCreateTexture.Invoke(desc)
	return NewTexture(jsTexture)
}

// CreateSamplerFromDesc creates a GPUSampler from a JS descriptor object.
func (d *Device) CreateSamplerFromDesc(desc js.Value) *Sampler {
	jsSampler := d.fnCreateSampler.Invoke(desc)
	return NewSampler(jsSampler)
}

// CreateShaderModuleFromDesc creates a GPUShaderModule from a JS descriptor object.
func (d *Device) CreateShaderModuleFromDesc(desc js.Value) *ShaderModule {
	jsModule := d.fnCreateShaderModule.Invoke(desc)
	return NewShaderModule(jsModule)
}

// CreateBindGroupLayoutFromDesc creates a GPUBindGroupLayout from a JS descriptor object.
func (d *Device) CreateBindGroupLayoutFromDesc(desc js.Value) *BindGroupLayout {
	jsLayout := d.fnCreateBindGroupLayout.Invoke(desc)
	return NewBindGroupLayout(jsLayout)
}

// CreateBindGroupFromDesc creates a GPUBindGroup from a JS descriptor object.
func (d *Device) CreateBindGroupFromDesc(desc js.Value) *BindGroup {
	jsGroup := d.fnCreateBindGroup.Invoke(desc)
	return NewBindGroup(jsGroup)
}

// CreatePipelineLayoutFromDesc creates a GPUPipelineLayout from a JS descriptor object.
func (d *Device) CreatePipelineLayoutFromDesc(desc js.Value) *PipelineLayout {
	jsLayout := d.fnCreatePipelineLayout.Invoke(desc)
	return NewPipelineLayout(jsLayout)
}

// CreateRenderPipelineFromDesc creates a GPURenderPipeline from a JS descriptor object.
func (d *Device) CreateRenderPipelineFromDesc(desc js.Value) *RenderPipeline {
	jsPipeline := d.fnCreateRenderPipeline.Invoke(desc)
	return NewRenderPipeline(jsPipeline)
}

// CreateComputePipelineFromDesc creates a GPUComputePipeline from a JS descriptor object.
func (d *Device) CreateComputePipelineFromDesc(desc js.Value) *ComputePipeline {
	jsPipeline := d.fnCreateComputePipeline.Invoke(desc)
	return NewComputePipeline(jsPipeline)
}

// Destroy calls GPUDevice.destroy() to release GPU resources.
// After this call the device is no longer usable.
func (d *Device) Destroy() {
	destroy := d.ref_.Get("destroy")
	if !destroy.IsUndefined() && !destroy.IsNull() {
		d.ref_.Call("destroy")
	}
}
